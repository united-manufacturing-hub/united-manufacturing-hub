//go:build kafka
// +build kafka

package internal

import (
	"github.com/go-playground/assert/v2"
	"testing"
)

func TestGTICount(t *testing.T) {
	count := "ia.a.b.c.count"
	countGTI := GetTopicInformationCached(count)
	assert.NotEqual(t, countGTI, nil)
	assert.Equal(t, countGTI.CustomerId, "a")
	assert.Equal(t, countGTI.Location, "b")
	assert.Equal(t, countGTI.AssetId, "c")
	assert.Equal(t, countGTI.Topic, "count")
	assert.Equal(t, len(countGTI.ExtendedTopics), 0)
	assert.Equal(t, countGTI.TransmitterId, nil)
	assert.Equal(t, countGTI.MacAddressOfCamera, nil)
}

func TestGTICountLongerNames(t *testing.T) {
	count := "ia.abc.def.gih.count"
	countGTI := GetTopicInformationCached(count)
	assert.NotEqual(t, countGTI, nil)
	assert.Equal(t, countGTI.CustomerId, "abc")
	assert.Equal(t, countGTI.Location, "def")
	assert.Equal(t, countGTI.AssetId, "gih")
	assert.Equal(t, countGTI.Topic, "count")
	assert.Equal(t, len(countGTI.ExtendedTopics), 0)
	assert.Equal(t, countGTI.TransmitterId, nil)
	assert.Equal(t, countGTI.MacAddressOfCamera, nil)
}

func TestGTIRaw(t *testing.T) {
	raw := "ia.raw.a"
	rawGTI := GetTopicInformationCached(raw)
	assert.NotEqual(t, rawGTI, nil)
	assert.Equal(t, rawGTI.CustomerId, "")
	assert.Equal(t, rawGTI.Location, "")
	assert.Equal(t, rawGTI.AssetId, "")
	assert.Equal(t, rawGTI.Topic, "raw")
	assert.Equal(t, rawGTI.ExtendedTopics, []string{"a"})
	assert.Equal(t, rawGTI.TransmitterId, nil)
	assert.Equal(t, rawGTI.MacAddressOfCamera, nil)
}

func TestGTIRawImage(t *testing.T) {
	rawImage := "ia.rawImage.a.b"
	rawImageGTI := GetTopicInformationCached(rawImage)
	assert.NotEqual(t, rawImageGTI, nil)
	assert.Equal(t, rawImageGTI.CustomerId, "")
	assert.Equal(t, rawImageGTI.Location, "")
	assert.Equal(t, rawImageGTI.AssetId, "")
	assert.Equal(t, rawImageGTI.Topic, "rawImage")
	assert.Equal(t, len(rawImageGTI.ExtendedTopics), 0)
	assert.Equal(t, *rawImageGTI.TransmitterId, "a")
	assert.Equal(t, *rawImageGTI.MacAddressOfCamera, "b")
}
func TestGTIProcessValue(t *testing.T) {
	count := "ia.a.b.c.processValue"
	countGTI := GetTopicInformationCached(count)
	assert.NotEqual(t, countGTI, nil)
	assert.Equal(t, countGTI.CustomerId, "a")
	assert.Equal(t, countGTI.Location, "b")
	assert.Equal(t, countGTI.AssetId, "c")
	assert.Equal(t, countGTI.Topic, "processValue")
	assert.Equal(t, len(countGTI.ExtendedTopics), 0)
	assert.Equal(t, countGTI.TransmitterId, nil)
	assert.Equal(t, countGTI.MacAddressOfCamera, nil)
}
func TestGTIProcessValueABC(t *testing.T) {
	count := "ia.a.b.c.processValue.a.b.c"
	countGTI := GetTopicInformationCached(count)
	assert.NotEqual(t, countGTI, nil)
	assert.Equal(t, countGTI.CustomerId, "a")
	assert.Equal(t, countGTI.Location, "b")
	assert.Equal(t, countGTI.AssetId, "c")
	assert.Equal(t, countGTI.Topic, "processValue")
	assert.Equal(t, len(countGTI.ExtendedTopics), 3)
	assert.Equal(t, countGTI.TransmitterId, nil)
	assert.Equal(t, countGTI.MacAddressOfCamera, nil)
}

func BenchmarkRegex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getTopicInformation("ia.a.b.c.count")
		getTopicInformation("ia.raw.a")
		getTopicInformation("ia.rawImage.a.b")
		getTopicInformation("ia.a.b.c.INVALID")
	}
}

func BenchmarkCachedRegex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetTopicInformationCached("ia.a.b.c.count")
		GetTopicInformationCached("ia.raw.a")
		GetTopicInformationCached("ia.rawImage.a.b")
		GetTopicInformationCached("ia.a.b.c.INVALID")
	}

}
