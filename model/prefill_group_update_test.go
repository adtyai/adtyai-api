package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdatePrefillGroupKeepsCreatedTime(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&PrefillGroup{}))
	group := &PrefillGroup{Name: "preset", Type: "model", Items: JSONValue(`["gpt"]`), Description: "first"}
	require.NoError(t, group.Insert())
	require.NotZero(t, group.CreatedTime)

	edited := &PrefillGroup{Id: group.Id, Name: "preset", Type: "model", Items: JSONValue(`["gpt"]`), Description: "second"}
	require.NoError(t, edited.Update())
	var stored PrefillGroup
	require.NoError(t, db.First(&stored, group.Id).Error)
	require.Equal(t, group.CreatedTime, stored.CreatedTime)
	require.Equal(t, "second", stored.Description)

	missing := &PrefillGroup{Id: group.Id + 1000, Name: "other", Type: "model"}
	require.Error(t, missing.Update())
	var count int64
	require.NoError(t, db.Model(&PrefillGroup{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
