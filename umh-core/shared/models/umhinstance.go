package models

// Importing the UUID library from Google for several reasons:
// - While strings could theoretically be used for UUIDs, using a dedicated type offers additional type safety.
// - The library is well-established with over 4.6k stars on GitHub and ensures standard compliance.
import (
	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
)

type InstanceName string

// This needs to be here as it would otherwise create circular dependencies
// database.go as well as the UNS require this
const (
	// Instance1name is a test instance !
	// Deprecated: Use Instance4name or higher
	Instance1name InstanceName = "Instance1"
	// Instance2name is a test instance !
	// Deprecated: Use Instance4name or higher
	Instance2name InstanceName = "Instance2"
	// Instance3name is a test instance !
	// Deprecated: Use Instance4name or higher
	Instance3name InstanceName = "Instance3"

	// Instance4name is a test instance !
	// This instance is always lagging one version behind the latest version
	Instance4name InstanceName = "Visby"
	// Instance5name is a test instance !
	// This instance has a bad connection and is quite slow
	Instance5name InstanceName = "Riga"
	// Instance6name is a test instance !
	// This instance is always offline
	Instance6name InstanceName = "London"
	// Instance7name is a test instance !
	// This instance has an S7 connection, and is otherwise always perfectly fine, but it has a slow network connection
	Instance7name InstanceName = "Hamburg"
	// Instance8name is a test instance !
	// This instance has some opcua connections, and is otherwise always perfectly fine
	Instance8name InstanceName = "Cologne"
	// Instance9name is a test instance !
	// This instance is always perfectly fine, but is in a different enterprise
	Instance9name InstanceName = "Bruges"
	// Instance10name is a test instance !
	// This instance is always perfectly fine, but it's a lite instance
	Instance10name InstanceName = "Gdańsk"
	// Instance11name is a test instance !
	// This instance is always perfectly fine, but it was a lite instance that later upgraded
	Instance11name InstanceName = "Tallinn"
	// Instance12name is a test instance !
	// This instance is always perfectly fine, but it's a lite instance that is one version behind
	Instance12name InstanceName = "Lübeck"
	// Instance13name is a test instance !
	// This instance has problems connecting to the OPCUA server
	Instance13name InstanceName = "Bremen"
	// Instance14name is a test instance !
	// This instance sends data from a location it isn't supposed to
	Instance14name InstanceName = "Stralsund"
	// Instance15name is a test instance !
	// This instance is always perfectly fine, but it sends data from Site Visby (Instance4name)
	Instance15name InstanceName = "Bergen"
	// Instance16name is a test instance !
	// This instance is always perfectly fine, but it's data has schema validation errors (and sometimes sends broken JSON as well)
	Instance16name InstanceName = "Vilnius"
)

// UMHInstance represents a unique instance of a UMH instance.
type UMHInstance struct {
	Name             InstanceName    `json:"name"`        // The name of the UMH instance
	UUID             uuid.UUID       `json:"uuid"`        // The UUID of the UMH instance, for unique identification
	CompanyUUID      uuid.UUID       `json:"companyUUID"` // The UUID of the company that owns the UMH instance
	Verified         bool            `json:"verified"`    // Whether the UMH instance is verified
	IsDeleted        bool            `json:"isDeleted"`   // Whether the UMH instance is deleted
	Version          *semver.Version `json:"version"`
	Certificate      *string         `json:"certificate"`
	EncryptedPrivKey *string         `json:"encryptedPrivateKey"`
}
