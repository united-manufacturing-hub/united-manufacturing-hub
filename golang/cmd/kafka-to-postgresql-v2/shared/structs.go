package shared

type TopicDetails struct {
	Enterprise     string
	Site           string
	Area           string
	ProductionLine string
	WorkCell       string
	OriginId       string
	Usecase        string
	Tag            string
}

type Value struct {
	NumericValue *float32
	StringValue  *string
	Name         string
	IsNumeric    bool
}
