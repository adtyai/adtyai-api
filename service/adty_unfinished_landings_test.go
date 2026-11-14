package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerEarningLineShowsModelAndGroupWithoutChannel(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, RecordAdtySiteEarningLine(7, 9, "gpt-test", "vip", "req-1", 500000, 1.25, 0.8))
	rows, err := ListAdtyOwnerEarningLines(7, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "gpt-test", rows[0].ModelName)
	assert.Equal(t, "vip", rows[0].Group)
	assert.Equal(t, "req-1", rows[0].RequestId)
	body, err := common.Marshal(rows[0])
	require.NoError(t, err)
	assert.NotContains(t, string(body), "channel")
	assert.NotContains(t, string(body), "upstream")
	var stored int64
	require.NoError(t, db.Model(&model.AdtySiteEarningLine{}).Count(&stored).Error)
	assert.EqualValues(t, 1, stored)
}

func TestAnomalyReviewNotifiesTheSite(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "anomaly-note-owner")
	require.NoError(t, recordAdtyAnomalyDisposition("balance_ledger_gap", strconv.Itoa(4), "open", "detected", 0))
	var row model.AdtyAnomalyDisposition
	require.NoError(t, db.First(&row).Error)
	require.NoError(t, ReviewAdtyAnomalyDisposition(row.Id, owner.Id, "closed", "checked the ledger"))
	var notices []model.AdtyNotificationPublication
	require.NoError(t, db.Where("audience = ? AND target_site_id = ?", "site", 4).Find(&notices).Error)
	require.NotEmpty(t, notices, "closing a fund anomaly must notify the site")
	assert.NotContains(t, notices[0].Content, "channel")
	assert.Contains(t, notices[0].Content, "checked the ledger")
	assert.Contains(t, notices[0].Content, "资金异常处置已记录")
}
