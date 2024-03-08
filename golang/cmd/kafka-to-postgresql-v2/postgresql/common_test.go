package postgresql

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
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

func TestCreateMockConnection(t *testing.T) {
	c := CreateMockConnection(t)
	assert.NotNil(t, c)
	assert.NotNil(t, c.db)
	assert.NotNil(t, c.assetIdCache)
	assert.NotNil(t, c.productTypeIdCache)
	assert.NotNil(t, c.numericalValuesChannel)
	assert.NotNil(t, c.stringValuesChannel)
}
