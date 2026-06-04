package domain

// FeatureFlagName is the typed identifier for a feature toggle. Constants
// below name every flag the application reads. The repository persists the
// underlying string, but callers exchange the typed value so a misspelled
// name is a compile error.
type FeatureFlagName string

// Feature flag names known to the application. Add a new constant when you
// add a new flag; the repository will store the underlying string verbatim.
const (
	FeatureFlagMaintenanceMode FeatureFlagName = "maintenance_mode"
)

// FeatureFlag toggles application features at runtime.
type FeatureFlag struct {
	Name    FeatureFlagName
	Enabled bool
}
