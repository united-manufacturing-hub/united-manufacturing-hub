package mgmtconfig

type ReleaseChannel string

const (
	Unset      = ""
	Enterprise = "enterprise"
	Stable     = "stable"
	Nightly    = "nightly"
)

type UmhConfig struct {
	Version               string         `yaml:"version"`
	HelmChartAddon        string         `yaml:"helm_chart_addon"`
	UmhMergePoint         int            `yaml:"umh_merge_point"`
	LastUpdated           uint64         `yaml:"lastUpdated"`
	Enabled               bool           `yaml:"enabled"`
	ConvertingActionsDone bool           `yaml:"convertingActionsDone"`
	ReleaseChannel        ReleaseChannel `yaml:"releaseChannel"`

	// DisableHardwareStatusCheck is a flag to disable the hardware status check inside the status message
	// It is used for testing purposes
	DisableHardwareStatusCheck *bool `yaml:"disableHardwareStatusCheck"`
}
