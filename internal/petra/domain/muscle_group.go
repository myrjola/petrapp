package domain

// MuscleGroupTarget stores the weekly hard-set range for a tracked muscle
// group: MinSets is the floor (≈ MEV, minimum effective volume) the planner
// drives toward, MaxSets the ceiling (≈ MRV, maximum recoverable volume)
// beyond which extra volume is penalised. The bounds are authored in whole
// sets but compared against accumulated set credit, so a primary set counts 1
// toward the target and a secondary ½ (see PrimarySetCredit/SecondarySetCredit).
type MuscleGroupTarget struct {
	MuscleGroupName string
	MinSets         int
	MaxSets         int
}

// MuscleGroupVolume captures the weekly weighted set credit for a single muscle group.
// Each set in the plan contributes to every muscle group it touches: PrimarySetCredit
// for primaries and SecondarySetCredit for secondaries. Completed counts only sets
// that have a CompletedAt timestamp; Planned counts every set in the weekly plan and
// is therefore always >= Completed. TargetSets is 0 for muscle groups that don't
// have a row in muscle_group_weekly_targets.
type MuscleGroupVolume struct {
	Name            string
	CompletedCredit float64
	PlannedCredit   float64
	TargetSets      int
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

// PrimarySetCredit and SecondarySetCredit are the per-set credit toward a
// muscle group's weekly volume. The split reflects that secondary engagement
// receives meaningfully less stimulus than primary engagement.
const (
	PrimarySetCredit   = 1.0
	SecondarySetCredit = 0.5
)

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly credit per
// muscle group across the supplied sessions. One entry is returned for
// every muscle group in groupNames, sorted to match groupNames' order.
// Groups with no contributions appear as zero-credit rows so callers can
// render them without a separate query. Targets are joined from the targets
// slice; muscle groups missing from targets carry TargetSets = 0.
func WeeklyMuscleGroupVolume(
	sessions []Session,
	targets []MuscleGroupTarget,
	groupNames []string,
) []MuscleGroupVolume {
	targetByName := make(map[string]int, len(targets))
	for _, t := range targets {
		targetByName[t.MuscleGroupName] = t.MinSets
	}

	known := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		known[name] = struct{}{}
	}

	planned := make(map[string]float64, len(groupNames))
	completed := make(map[string]float64, len(groupNames))
	aggregateMuscleGroupCredit(sessions, known, planned, completed)

	result := make([]MuscleGroupVolume, 0, len(groupNames))
	for _, name := range groupNames {
		result = append(result, MuscleGroupVolume{
			Name:            name,
			CompletedCredit: completed[name],
			PlannedCredit:   planned[name],
			TargetSets:      targetByName[name],
		})
	}
	return result
}

// WeeklyPlannedCredit returns the running planned credit per
// muscle group across the supplied sessions. Each set in the plan
// contributes PrimarySetCredit to every primary muscle group on its
// exercise and SecondarySetCredit to every secondary. Muscle groups
// with zero contributions do not appear in the map. The result is the
// running tally the target-aware planner uses to score subsequent
// picks against the configured weekly targets.
func WeeklyPlannedCredit(sessions []Session) map[string]float64 {
	credit := make(map[string]float64)
	for _, sess := range sessions {
		for _, ex := range sess.Slots {
			n := float64(len(ex.Sets))
			for _, mg := range ex.Exercise.PrimaryMuscleGroups {
				credit[mg] += n * PrimarySetCredit
			}
			for _, mg := range ex.Exercise.SecondaryMuscleGroups {
				credit[mg] += n * SecondarySetCredit
			}
		}
	}
	return credit
}

// aggregateMuscleGroupCredit walks every set in the supplied sessions and totals the
// credit for each muscle group, accumulating into the planned and completed
// maps. Primary contributions count as PrimarySetCredit, secondary as
// SecondarySetCredit. Muscle group names not present in known are silently skipped
// — they cannot occur in production due to FK constraints, but the guard keeps
// tests safe when synthetic exercises reference unknown groups.
func aggregateMuscleGroupCredit(
	sessions []Session,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, sess := range sessions {
		for _, ex := range sess.Slots {
			for _, set := range ex.Sets {
				done := set.CompletedAt != nil
				creditMuscleGroups(ex.Exercise.PrimaryMuscleGroups, PrimarySetCredit, done, known, planned, completed)
				creditMuscleGroups(
					ex.Exercise.SecondaryMuscleGroups,
					SecondarySetCredit,
					done,
					known,
					planned,
					completed,
				)
			}
		}
	}
}

// creditMuscleGroups credits weight to each muscle group in names, both to planned
// and (when done) to completed. Groups missing from known are ignored.
func creditMuscleGroups(
	names []string,
	weight float64,
	done bool,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, mg := range names {
		if _, ok := known[mg]; !ok {
			continue
		}
		planned[mg] += weight
		if done {
			completed[mg] += weight
		}
	}
}
