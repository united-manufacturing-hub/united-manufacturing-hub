package generator

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
)

const (
	defaultCacheExpiration = 1 * time.Hour
	defaultCacheCullPeriod = 10 * time.Minute
)

type StatusCollectorType struct {
	latestData *LatestData
	dog        watchdog.Iface
}

type LatestData struct {
	mu sync.RWMutex // A mutex to synchronize access to the fields

	UnsTable   models.UnsTable
	EventTable models.EventTable
}

func NewStatusCollector(
	dog watchdog.Iface,
) *StatusCollectorType {

	latestData := &LatestData{}

	collector := &StatusCollectorType{
		latestData: latestData,
		dog:        dog,
	}

	return collector
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	// mock the status message

}
