package generator

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
)

const (
	defaultCacheExpiration = 1 * time.Hour
	defaultCacheCullPeriod = 10 * time.Minute
)

type StatusCollectorType struct {
	latestData *LatestData
	dog        watchdog.Iface
	config     *config.FullConfig
}

type LatestData struct {
	mu sync.RWMutex // A mutex to synchronize access to the fields

	UnsTable   models.UnsTable
	EventTable models.EventTable
}

func NewStatusCollector(
	dog watchdog.Iface,
	config *config.FullConfig,
) *StatusCollectorType {

	latestData := &LatestData{}

	collector := &StatusCollectorType{
		latestData: latestData,
		dog:        dog,
		config:     config,
	}

	return collector
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	return nil
}
