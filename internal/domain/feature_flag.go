package domain

// FeatureFlag toggles application features at runtime.
type FeatureFlag struct {
	Name    string
	Enabled bool
}
