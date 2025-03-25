package mgmtconfig

type DebugConfig struct {
	DisableBackendConnection bool   `yaml:"disableBackendConnection"`
	UpdateTagOverwrite       string `yaml:"updaterImageOverwrite"`
}
