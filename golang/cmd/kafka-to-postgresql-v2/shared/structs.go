package shared

const (
	DbTagSeparator = "$"
)
type TopicDetails struct {
	Enterprise     string
	Site           string
	Area           string
	ProductionLine string
	WorkCell       string
	OriginId       string
	Schema         string
	Tag            string
}

type Value struct {
	NumericValue *float32
	StringValue  *string
	IsNumeric    bool
	Name         string
}
