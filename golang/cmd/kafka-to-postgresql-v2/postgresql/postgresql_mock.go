package postgresql

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func CreateMockConnection(t *testing.T) *Connection {
	// Passing t here to ensure it is not used in production code

	// AssetIdCache
	assetIdCache, err := lru.NewARC(10)
	if err != nil {
		t.Fatalf("Failed to create AssetIdCache: %v", err)
	}

	// ProductTypeIdCache
	productTypeIdCache, err := lru.NewARC(10)
	if err != nil {
		t.Fatalf("Failed to create ProductTypeIdCache: %v", err)
	}

	mocked, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("Failed to create mock connection: %v", err)
	}

	c := Connection{
		Db:                     mocked,
		AssetIdCache:           assetIdCache,
		ProductTypeIdCache:     productTypeIdCache,
		NumericalValuesChannel: make(chan shared.DBHistorianValue, 100),
		StringValuesChannel:    make(chan shared.DBHistorianValue, 100),
	}

	c.Db = mocked
	return &c
}
