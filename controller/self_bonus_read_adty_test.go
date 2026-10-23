package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildSelfUserDataOmitsBonusWhenReadFails(t *testing.T) {
	prevDB, prevLog := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
	})

	data := buildSelfUserData(&model.User{
		Id:       7,
		Username: "reader",
		Quota:    1000,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	})
	_, ok := data["bonus_quota"]
	require.False(t, ok)
	require.Equal(t, 1000, data["quota"])
}

func TestBuildSelfUserDataKeepsZeroBonusWhenReadSucceeds(t *testing.T) {
	prevDB, prevLog := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserBonusWallet{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLog
	})

	data := buildSelfUserData(&model.User{
		Id:       7,
		Username: "reader",
		Quota:    1000,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	})
	require.Equal(t, int64(0), data["bonus_quota"])
	require.Equal(t, 1000, data["quota"])
}
