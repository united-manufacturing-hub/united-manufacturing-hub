package mgmtconfig

import (
	uuidlib "github.com/google/uuid"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/ctypes"
)

type Connection struct {
	Name        string                          `yaml:"name"`
	Ip          string                          `yaml:"ip"`
	Type        ctypes.DatasourceConnectionType `yaml:"type"`
	Notes       string                          `yaml:"notes"`
	Port        int                             `yaml:"port"`
	LastUpdated uint64                          `yaml:"lastUpdated"`
	Uuid        uuidlib.UUID                    `yaml:"uuid"`
	Location    *ConnectionLocation             `yaml:"location"` // This is optional for backward compatibility
}

type ConnectionLocation struct {
	Enterprise          string  `yaml:"enterprise" json:"enterprise"` // Always inherited
	Site                *string `yaml:"site" json:"site"`
	SiteIsInherited     bool    `yaml:"siteIsInherited" json:"siteIsInherited"`
	Area                *string `yaml:"area" json:"area"`
	AreaIsInherited     bool    `yaml:"areaIsInherited" json:"areaIsInherited"`
	Line                *string `yaml:"line" json:"line"`
	LineIsInherited     bool    `yaml:"lineIsInherited" json:"lineIsInherited"`
	WorkCell            *string `yaml:"workCell" json:"workCell"`
	WorkCellIsInherited bool    `yaml:"workCellIsInherited" json:"workCellIsInherited"`
}
