package worker

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseHistorianPayloadValNameEqualsTagname(t *testing.T) {
	payload := `{"timestamp_ms": 1600000000000, "val":10000}`
	tag := "val"

	histValues, timestamp, err := parseHistorianPayload([]byte(payload), tag)
	assert.NoError(t, err)
	assert.Equal(t, 1600000000000, int(timestamp))
	assert.Equal(t, 1, len(histValues))
	firstValue := histValues[0]
	assert.Equal(t, "val$val", firstValue.Name)

}
func TestParseHistorianPayloadValNameNotEqualsTagname(t *testing.T) {
	payload := `{"timestamp_ms": 1600000000000, "val":10000}`
	tag := "1.2.3.4"

	histValues, timestamp, err := parseHistorianPayload([]byte(payload), tag)
	assert.NoError(t, err)
	assert.Equal(t, 1600000000000, int(timestamp))
	assert.Equal(t, 1, len(histValues))
	firstValue := histValues[0]
	assert.Equal(t, "1$2$3$4$val", firstValue.Name)
}

func TestParseHistorianPayloadValNameNotEqualsTagnameComplex(t *testing.T) {
	payload := `{"timestamp_ms": 1600000000000, "val":{ "val":10000}}`
	tag := "1.2.3.4"

	histValues, timestamp, err := parseHistorianPayload([]byte(payload), tag)
	assert.NoError(t, err)
	assert.Equal(t, 1600000000000, int(timestamp))
	assert.Equal(t, 1, len(histValues))
	firstValue := histValues[0]
	assert.Equal(t, "1$2$3$4$val$val", firstValue.Name)
}
