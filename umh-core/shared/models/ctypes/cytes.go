package ctypes

// DatasourceConnectionType specifies the type of data source connection.
type DatasourceConnectionType string

type Protocol string

const (
	// Deprecated: Use a data flow component instead.
	BenthosOPCUA Protocol = "benthos_opcua"
	// Deprecated: Use a data flow component instead.
	BenthosGeneric Protocol = "benthos_generic"
)
