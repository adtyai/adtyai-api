package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchSetChannelTagRewritesAbilityTag(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AdtyGroup{}))
	oldTag := "old-tag"
	channel := &Channel{
		Name: "tag-batch", Type: 1, Key: "sk-tag-batch", Status: common.ChannelStatusEnabled,
		Models: "gpt-tag", Group: "default", Tag: &oldTag,
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		DB.Where("channel_id = ?", channel.Id).Delete(&Ability{})
		DB.Unscoped().Delete(&Channel{}, channel.Id)
	})

	var before []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&before).Error)
	require.NotEmpty(t, before)
	require.NotNil(t, before[0].Tag)
	assert.Equal(t, oldTag, *before[0].Tag)

	newTag := "new-tag"
	require.NoError(t, BatchSetChannelTag([]int{channel.Id}, &newTag))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.NotNil(t, stored.Tag)
	assert.Equal(t, newTag, *stored.Tag)

	var after []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&after).Error)
	require.NotEmpty(t, after)
	for _, ability := range after {
		require.NotNil(t, ability.Tag)
		assert.Equal(t, newTag, *ability.Tag, "ability %s/%s", ability.Group, ability.Model)
	}
}
