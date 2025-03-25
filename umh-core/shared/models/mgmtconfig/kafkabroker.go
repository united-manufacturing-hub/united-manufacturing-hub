package mgmtconfig

type KafkaBroker struct {
	Ip   string `yaml:"ip"`
	Port int    `yaml:"port"`
}
