package models

type PromMetrics map[string][]PromMetric
type PromMetric struct {
	Path   string
	Labels map[string]string
	Value  float64
}
