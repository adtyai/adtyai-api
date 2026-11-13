package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShelvedCatalogModelIsNotCallable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Model{}))
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })

	live := Model{ModelName: "live-model", Status: 1}
	hidden := Model{ModelName: "hidden-model", Status: 1}
	require.NoError(t, db.Create(&live).Error)
	require.NoError(t, db.Create(&hidden).Error)
	require.NoError(t, db.Model(&hidden).Update("status", 0).Error)

	assert.True(t, CatalogModelCallable("live-model"))
	assert.False(t, CatalogModelCallable("hidden-model"))
	assert.True(t, CatalogModelCallable("channel-only-model"))
}
