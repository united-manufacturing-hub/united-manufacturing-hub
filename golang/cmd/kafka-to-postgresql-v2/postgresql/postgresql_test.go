package postgresql

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateMockConnection(t *testing.T) {
	c := CreateMockConnection(t)
	assert.NotNil(t, c)
	assert.NotNil(t, c.Db)
	assert.NotNil(t, c.AssetIdCache)
	assert.NotNil(t, c.ProductTypeIdCache)
	assert.NotNil(t, c.NumericalValuesChannel)
	assert.NotNil(t, c.StringValuesChannel)
}
