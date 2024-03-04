package postgresql

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/pashagolub/pgxmock/v3"
	"testing"
)

func CreateMockConnection(t *testing.T) *Connection {
	var c Connection

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

	c.assetIdCache = assetIdCache
	c.productTypeIdCache = productTypeIdCache
	c.numericalValuesChannel = make(chan DBValue, 100)
	c.stringValuesChannel = make(chan DBValue, 100)

	mocked, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("Failed to create mock connection: %v", err)
	}
	c.db = mocked
	return &c
}
