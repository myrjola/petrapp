package domain

// MuscleGroupTarget stores the weekly hard-set range for a tracked muscle
// group: MinSets is the floor (≈ MEV, minimum effective volume) the planner
// drives toward, MaxSets the ceiling (≈ MRV, maximum recoverable volume)
// beyond which extra volume is penalised. The bounds are authored in whole
// sets but compared against accumulated volume, so a primary set counts as a
// full set toward the target and a secondary as a fractional ½ set (see
// PrimarySetFraction/SecondarySetFraction).
type MuscleGroupTarget struct {
	MuscleGroupName string
	MinSets         int
	MaxSets         int
}

// MuscleGroupVolume captures a muscle group's weekly volume, summed in fractional sets.
// Each set in the plan contributes to every muscle group it touches: PrimarySetFraction
// for primaries and SecondarySetFraction for secondaries. Completed counts only sets
// that have a CompletedAt timestamp; Planned counts every set in the weekly plan and
// is therefore always >= Completed. MinSets/MaxSets carry the muscle group's target
// band (≈ MEV/MRV) and are 0 for muscle groups that don't have a row in
// muscle_group_weekly_targets.
type MuscleGroupVolume struct {
	Name            string
	CompletedVolume float64
	PlannedVolume   float64
	MinSets         int
	MaxSets         int
}

// MuscleGroupVolumeStatus classifies a muscle group's planned weekly volume
// against its target band. The string values double as CSS state tokens on the
// muscle-balance bars.
type MuscleGroupVolumeStatus string

const (
	MuscleVolumeNoTarget MuscleGroupVolumeStatus = "no-target"
	MuscleVolumeUnder    MuscleGroupVolumeStatus = "under"
	MuscleVolumeOnTarget MuscleGroupVolumeStatus = "on-target"
	MuscleVolumeOver     MuscleGroupVolumeStatus = "over"
)

// Status classifies the planned weekly volume against the MinSets…MaxSets
// band: under below the floor (≈ MEV), on-target inside the band, over above
// the ceiling (≈ MRV) — the same band the planner's set-count ramp climbs, so
// the display never flags the planner's own late-cycle prescription as
// excessive. Muscle groups without a seeded target are no-target so the UI
// can render them informationally without a value judgment.
func (v MuscleGroupVolume) Status() MuscleGroupVolumeStatus {
	switch {
	case v.MinSets <= 0:
		return MuscleVolumeNoTarget
	case v.PlannedVolume < float64(v.MinSets):
		return MuscleVolumeUnder
	case v.PlannedVolume <= float64(v.MaxSets):
		return MuscleVolumeOnTarget
	default:
		return MuscleVolumeOver
	}
}

// MuscleGroupRegion is a coarse anatomical grouping used by UI layers to arrange
// the per-muscle-group bars into push/pull/legs/core sections.
type MuscleGroupRegion string

const (
	RegionUpperPush MuscleGroupRegion = "Upper Push"
	RegionUpperPull MuscleGroupRegion = "Upper Pull"
	RegionLegs      MuscleGroupRegion = "Legs"
	RegionCore      MuscleGroupRegion = "Core"
	RegionOther     MuscleGroupRegion = "Other"
)

// Muscle-group names are the canonical identifiers shared by MuscleGroupTarget,
// Exercise muscle-group fields, and the planner. They mirror the rows seeded in
// the muscle_groups table.
const (
	MuscleGroupChest      = "Chest"
	MuscleGroupShoulders  = "Shoulders"
	MuscleGroupSideDelts  = "Side Delts"
	MuscleGroupRearDelts  = "Rear Delts"
	MuscleGroupTriceps    = "Triceps"
	MuscleGroupUpperBack  = "Upper Back"
	MuscleGroupLats       = "Lats"
	MuscleGroupBiceps     = "Biceps"
	MuscleGroupTraps      = "Traps"
	MuscleGroupForearms   = "Forearms"
	MuscleGroupQuads      = "Quads"
	MuscleGroupHamstrings = "Hamstrings"
	MuscleGroupGlutes     = "Glutes"
	MuscleGroupCalves     = "Calves"
	MuscleGroupAdductors  = "Adductors"
	MuscleGroupAbs        = "Abs"
	MuscleGroupObliques   = "Obliques"
	MuscleGroupLowerBack  = "Lower Back"
)

// RegionFor classifies a muscle group name into its anatomical region. Names that
// aren't recognised fall through to RegionOther so newly added muscle groups still
// render even before this map is updated.
func RegionFor(muscleGroupName string) MuscleGroupRegion {
	switch muscleGroupName {
	case MuscleGroupChest, MuscleGroupShoulders, MuscleGroupSideDelts, MuscleGroupTriceps:
		return RegionUpperPush
	case MuscleGroupUpperBack, MuscleGroupLats, MuscleGroupBiceps, MuscleGroupTraps,
		MuscleGroupForearms, MuscleGroupRearDelts:
		return RegionUpperPull
	case MuscleGroupQuads, MuscleGroupHamstrings, MuscleGroupGlutes,
		MuscleGroupCalves, MuscleGroupAdductors:
		return RegionLegs
	case MuscleGroupAbs, MuscleGroupObliques, MuscleGroupLowerBack:
		return RegionCore
	default:
		return RegionOther
	}
}

// PrimarySetFraction and SecondarySetFraction are the fraction of a set credited
// toward a muscle group's weekly volume: a primary muscle gets a full set, a
// secondary a fractional (½) set. The split reflects that secondary engagement
// receives meaningfully less stimulus than primary engagement.
const (
	PrimarySetFraction   = 1.0
	SecondarySetFraction = 0.5
)

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly volume per
// muscle group across the supplied sessions. One entry is returned for
// every muscle group in groupNames, sorted to match groupNames' order.
// Groups with no contributions appear as zero-volume rows so callers can
// render them without a separate query. Targets are joined from the targets
// slice; muscle groups missing from targets carry MinSets/MaxSets = 0.
func WeeklyMuscleGroupVolume(
	sessions []Session,
	targets []MuscleGroupTarget,
	groupNames []string,
) []MuscleGroupVolume {
	targetByName := make(map[string]MuscleGroupTarget, len(targets))
	for _, t := range targets {
		targetByName[t.MuscleGroupName] = t
	}

	known := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		known[name] = struct{}{}
	}

	planned := make(map[string]float64, len(groupNames))
	completed := make(map[string]float64, len(groupNames))
	aggregateMuscleGroupVolume(sessions, known, planned, completed)

	result := make([]MuscleGroupVolume, 0, len(groupNames))
	for _, name := range groupNames {
		result = append(result, MuscleGroupVolume{
			Name:            name,
			CompletedVolume: completed[name],
			PlannedVolume:   planned[name],
			MinSets:         targetByName[name].MinSets,
			MaxSets:         targetByName[name].MaxSets,
		})
	}
	return result
}

// WeeklyPlannedVolume returns the running planned volume per
// muscle group across the supplied sessions. Each set in the plan
// contributes PrimarySetFraction to every primary muscle group on its
// exercise and SecondarySetFraction to every secondary. Muscle groups
// with zero contributions do not appear in the map. The result is the
// running tally the target-aware planner uses to score subsequent
// picks against the configured weekly targets.
func WeeklyPlannedVolume(sessions []Session) map[string]float64 {
	volume := make(map[string]float64)
	for _, sess := range sessions {
		for _, ex := range sess.Slots {
			n := float64(len(ex.Sets))
			for _, mg := range ex.Exercise.PrimaryMuscleGroups {
				volume[mg] += n * PrimarySetFraction
			}
			for _, mg := range ex.Exercise.SecondaryMuscleGroups {
				volume[mg] += n * SecondarySetFraction
			}
		}
	}
	return volume
}

// aggregateMuscleGroupVolume walks every set in the supplied sessions and totals the
// volume for each muscle group, accumulating into the planned and completed
// maps. Primary contributions count as PrimarySetFraction, secondary as
// SecondarySetFraction. Muscle group names not present in known are silently skipped
// — they cannot occur in production due to FK constraints, but the guard keeps
// tests safe when synthetic exercises reference unknown groups.
func aggregateMuscleGroupVolume(
	sessions []Session,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, sess := range sessions {
		for _, ex := range sess.Slots {
			for _, set := range ex.Sets {
				done := set.CompletedAt != nil
				creditMuscleGroups(ex.Exercise.PrimaryMuscleGroups, PrimarySetFraction, done, known, planned, completed)
				creditMuscleGroups(
					ex.Exercise.SecondaryMuscleGroups,
					SecondarySetFraction,
					done,
					known,
					planned,
					completed,
				)
			}
		}
	}
}

// creditMuscleGroups credits volume to each muscle group in names, both to planned
// and (when done) to completed. Groups missing from known are ignored.
func creditMuscleGroups(
	names []string,
	fraction float64,
	done bool,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, mg := range names {
		if _, ok := known[mg]; !ok {
			continue
		}
		planned[mg] += fraction
		if done {
			completed[mg] += fraction
		}
	}
}
