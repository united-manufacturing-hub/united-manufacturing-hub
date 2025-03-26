package s6serviceconfig

import "reflect"

// S6ServiceConfig contains configuration for creating a service
type S6ServiceConfig struct {
	Command     []string          `yaml:"command"`
	Env         map[string]string `yaml:"env"`
	ConfigFiles map[string]string `yaml:"configFiles"`
	MemoryLimit int64             `yaml:"memoryLimit"` // 0 means no memory limit, see also https://skarnet.org/software/s6/s6-softlimit.html
}

// Equal checks if two S6ServiceConfigs are equal
func (c S6ServiceConfig) Equal(other S6ServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}
