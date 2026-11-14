package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureAdtySitePriceUniqueIndexMigratesLegacyIndex(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE adty_site_prices (
			id bigserial PRIMARY KEY,
			site_id int,
			user_id int,
			"group" varchar(64),
			markup decimal(10,4) NOT NULL DEFAULT 1,
			pending_markup decimal(10,4) NOT NULL DEFAULT 0,
			pending_at bigint,
			version bigint,
			updated_at bigint
		)
	`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_site_price ON adty_site_prices (site_id, user_id, "group")`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO adty_site_prices (site_id, user_id, "group", markup, pending_markup, version, updated_at)
		VALUES (1, 0, '', 1.2, 0, 1, 1)
	`).Error)

	require.NoError(t, db.AutoMigrate(&AdtySitePrice{}))
	require.NoError(t, EnsureAdtySitePriceUniqueIndex(db))
	require.NoError(t, EnsureAdtySitePriceUniqueIndex(db), "installing the index twice is a no-op")
	assert.True(t, db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceUniqueIndex))
	assert.False(t, db.Migrator().HasIndex(&AdtySitePrice{}, adtySitePriceLegacyIndex))

	var modelName *string
	require.NoError(t, db.Raw(`SELECT model FROM adty_site_prices WHERE site_id = 1`).Scan(&modelName).Error)
	require.NotNil(t, modelName)
	assert.Equal(t, "", *modelName)

	require.NoError(t, db.Create(&AdtySitePrice{
		SiteId: 1, UserId: 0, Group: "", Model: "gpt-4",
		Markup: decimal.RequireFromString("1.5"), Version: 1, UpdatedAt: 1,
	}).Error)

	err := db.Create(&AdtySitePrice{
		SiteId: 1, UserId: 0, Group: "", Model: "gpt-4",
		Markup: decimal.RequireFromString("1.8"), Version: 1, UpdatedAt: 1,
	}).Error
	require.Error(t, err, "the same site, user, group, and model cannot have two rows")

	err = db.Create(&AdtySitePrice{
		SiteId: 1, UserId: 0, Group: "", Model: "",
		Markup: decimal.RequireFromString("1.1"), Version: 1, UpdatedAt: 1,
	}).Error
	require.Error(t, err, "the legacy empty-model row remains unique")

	require.NoError(t, db.Create(&AdtySitePrice{
		SiteId: 1, UserId: 0, Group: "", Model: "claude-3",
		Markup: decimal.RequireFromString("1.1"), Version: 1, UpdatedAt: 1,
	}).Error)
}
