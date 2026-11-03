package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestDeleteDisabledChannelRemovesAbilities(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	disabled := &Channel{Name: "disabled-del", Key: "k", Status: common.ChannelStatusManuallyDisabled, Models: "m", Group: "default"}
	enabled := &Channel{Name: "enabled-keep", Key: "k2", Status: common.ChannelStatusEnabled, Models: "m", Group: "default"}
	require.NoError(t, db.Create(disabled).Error)
	require.NoError(t, db.Create(enabled).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "m", ChannelId: disabled.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "m2", ChannelId: enabled.Id, Enabled: true}).Error)

	rows, err := DeleteDisabledChannel()
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	var left int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", disabled.Id).Count(&left).Error)
	require.Zero(t, left)
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", enabled.Id).Count(&left).Error)
	require.Equal(t, int64(1), left)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", enabled.Id).Count(&left).Error)
	require.Equal(t, int64(1), left)
}
