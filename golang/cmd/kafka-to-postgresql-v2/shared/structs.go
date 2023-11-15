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
	NumericValue *float64
	StringValue  *string
	IsNumeric    bool
	Name         string
}
