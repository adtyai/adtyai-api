package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordSiteAuditTxWritesOfficialGlobalAuditLog(t *testing.T) {
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.AdtySiteAudit{}))
	require.NoError(t, model.MigrateAuditLogs())

	operator := model.User{Username: "site-admin", Password: "password-for-test", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&operator).Error)

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return recordSiteAuditTx(tx, 15, "adjust", operator.Id,
			map[string]any{"cost_factor": "1.10"},
			map[string]any{"cost_factor": "1.20"},
			"align supply price")
	}))

	var siteRow model.AdtySiteAudit
	require.NoError(t, model.DB.Where("site_id = ? AND action = ?", 15, "adjust").Take(&siteRow).Error)
	assert.Equal(t, operator.Id, siteRow.OperatorId)
	assert.Equal(t, "align supply price", siteRow.Reason)
	assert.Contains(t, siteRow.OldValue, "1.10")
	assert.Contains(t, siteRow.NewValue, "1.20")

	var stored model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ? AND object_id = ?", "adty.site.adjust", "15").Take(&stored).Error)
	require.NotNil(t, stored.SiteId)
	assert.Equal(t, 15, *stored.SiteId)
	assert.Equal(t, "site", stored.ObjectType)
	assert.Equal(t, "15", stored.ObjectId)
	assert.Equal(t, "adty.site.adjust", stored.Action)
	assert.Equal(t, model.AuditResultSuccess, stored.Result)
	assert.True(t, stored.Success)
	assert.Equal(t, operator.Id, stored.UserId)
	assert.Equal(t, "site-admin", stored.Username)
	assert.Equal(t, common.RoleAdminUser, stored.ActorRole)
	assert.Equal(t, "align supply price", stored.Reason)
	assert.Empty(t, stored.ApprovalRef)
	assert.NotEmpty(t, stored.RequestId)
	assert.Contains(t, stored.BeforeValue, "1.10")
	assert.Contains(t, stored.AfterValue, "1.20")
}
