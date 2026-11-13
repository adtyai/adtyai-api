package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimalGettersConvertAtBoundaryOnce(t *testing.T) {
	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":1.1}`))
	t.Cleanup(func() {
		_ = UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
	})

	got := GetGroupRatioDecimal("vip")
	assert.Equal(t, "1.1", got.String())
	assert.Equal(t, GetGroupRatio("vip"), got.InexactFloat64())
}
