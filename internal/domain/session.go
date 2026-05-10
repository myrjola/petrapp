package domain

// PeriodizationType is the rep-target style for a session. The two values
// alternate week-to-week (see Planner.firstSessionPeriodizationType) and
// determine the rep target via DeriveScheme.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)
