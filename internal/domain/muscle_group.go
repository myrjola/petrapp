package domain

// MuscleGroupTarget stores the minimum weekly set target for a tracked muscle group.
type MuscleGroupTarget struct {
	MuscleGroupName string
	WeeklySetTarget int
}

// MuscleGroupVolume captures the weekly weighted set load for a single muscle group.
// Each set in the plan contributes to every muscle group it touches: PrimarySetWeight
// for primaries and SecondarySetWeight for secondaries. Completed counts only sets
// that have a CompletedAt timestamp; Planned counts every set in the weekly plan and
// is therefore always >= Completed. TargetSets is 0 for muscle groups that don't
// have a row in muscle_group_weekly_targets.
type MuscleGroupVolume struct {
	Name          string
	CompletedLoad float64
	PlannedLoad   float64
	TargetSets    int
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

// RegionFor classifies a muscle group name into its anatomical region. Names that
// aren't recognised fall through to RegionOther so newly added muscle groups still
// render even before this map is updated.
func RegionFor(muscleGroupName string) MuscleGroupRegion {
	switch muscleGroupName {
	case "Chest", "Shoulders", "Triceps":
		return RegionUpperPush
	case "Upper Back", "Lats", "Biceps", "Traps", "Forearms":
		return RegionUpperPull
	case "Quads", "Hamstrings", "Glutes", "Calves", "Hip Flexors", "Adductors":
		return RegionLegs
	case "Abs", "Obliques", "Lower Back":
		return RegionCore
	default:
		return RegionOther
	}
}

// PrimarySetWeight and SecondarySetWeight are the per-set contributions to a
// muscle group's weekly load. The split reflects that secondary engagement
// receives meaningfully less stimulus than primary engagement.
const (
	PrimarySetWeight   = 1.0
	SecondarySetWeight = 0.5
)
