package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func officialFrontendPack(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "web", "src", "i18n", "locales", name))
	require.NoError(t, err)
	var file struct {
		Translation map[string]string `json:"translation"`
	}
	require.NoError(t, json.Unmarshal(data, &file))
	require.NotEmpty(t, file.Translation)
	return file.Translation
}

func TestSeedAdtyOfficialGlossaryProtectsA20Terms(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, SaveAdtyGlossaryTerm("永久赠金", "en", "coupon"))
	require.NoError(t, SeedAdtyOfficialGlossary())
	require.NoError(t, SeedAdtyOfficialGlossary())

	var terms []model.AdtyGlossaryTerm
	require.NoError(t, db.Find(&terms).Error)
	got := map[string]string{}
	for _, term := range terms {
		got[term.Locale+"\x00"+term.Source] = term.Target
	}
	require.Len(t, AdtyOfficialGlossary, 10)
	for _, pair := range AdtyOfficialGlossary {
		assert.Equal(t, pair.EN, got[AdtyOfficialGlossaryLocaleEN+"\x00"+pair.ZH], pair.ZH)
		assert.Equal(t, pair.ZH, got[AdtyOfficialGlossaryLocaleZhCN+"\x00"+pair.EN], pair.EN)
	}
	assert.Equal(t, "permanent bonus", got[AdtyOfficialGlossaryLocaleEN+"\x00永久赠金"])
}

func TestOfficialLocalePacksUseA20GlossaryTerms(t *testing.T) {
	en := officialFrontendPack(t, "en.json")
	zh := officialFrontendPack(t, "zh.json")
	require.Equal(t, len(en), len(zh))
	for key := range en {
		_, ok := zh[key]
		assert.True(t, ok, key)
	}

	official := map[string]string{
		"Principal":          "本金",
		"Permanent bonus":    "永久赠金",
		"Frozen amount":      "冻结金额",
		"Available Balance":  "可用余额",
		"Earnings":           "收益",
		"Deposit":            "保证金",
		"Manual refund":      "人工退款",
		"Commission release": "佣金释放",
		"Clawback":           "冲回",
		"Recovery":           "追偿",
	}
	for english, chinese := range official {
		assert.Equal(t, english, en[english], english)
		assert.Equal(t, chinese, zh[english], english)
	}

	assert.Equal(t, "Permanent bonus used", en["Bonus used"])
	assert.Equal(t, "已用永久赠金", zh["Bonus used"])
	assert.Equal(t, "Remaining permanent bonus", en["Bonus remaining"])
	assert.Equal(t, "剩余永久赠金", zh["Bonus remaining"])
	assert.Equal(t, "Manual refund", en["Refund order"])
	assert.Equal(t, "人工退款", zh["Refund order"])
	assert.Equal(t, "佣金冲回", zh["Commission clawback"])
	assert.Equal(t, "佣金部分冲回", zh["Partial commission clawback"])
	assert.Equal(t, "已冲回", zh["Clawed back"])
	assert.Equal(t, "Commission release", en["Commission voided"])
	assert.Equal(t, "佣金释放", zh["Commission voided"])
	assert.Equal(t, "冲回待结算和可提现的收益", zh["Claw back unsettled and withdrawable earnings"])

	refunded := "Refunded top-ups cancel or take back their commission."
	assert.Equal(t, "Refunded top-ups are handled as commission release, clawback, or recovery.", en[refunded])
	assert.Equal(t, "充值被退款时，对应的佣金会按状态释放、冲回或追偿。", zh[refunded])
	settles := "Commission settles {{days}} days after the top-up. Refunded top-ups void or claw back their commission, and the balance can go below zero."
	assert.Contains(t, en[settles], "commission release, clawback, or recovery")
	assert.Contains(t, zh[settles], "释放、冲回或追偿")
	assert.NotContains(t, zh[settles], "扣回")
	assert.NotContains(t, zh[settles], "追回")
	assert.NotContains(t, zh[refunded], "扣回")
	assert.NotContains(t, zh[refunded], "追回")

	assert.Equal(t, "钱包已扣回额度", zh["Wallet credit taken back"])
	assert.Equal(t, "已扣回本金额度", zh["Principal credit taken back"])
	assert.Equal(t, "链或地址填错无法追回。", zh["A wrong network or address cannot be recovered."])
}

func TestAdtyOfficialGlossaryRejectsRewrittenTerms(t *testing.T) {
	prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, SeedAdtyOfficialGlossary())
	_, err := SaveAdtyTranslationConfig(AdtyTranslationInput{Mode: model.AdtyTranslationModeSet, ModelName: "translator", Enabled: true, DailyQuota: 10})
	require.NoError(t, err)

	source := "永久赠金 12.50 USD，订单 A123456，不退款，已冲回"
	old := AdtyTranslateFunc
	AdtyTranslateFunc = func(_, _ string) (string, error) {
		return "Coupon 12.50 USD order A123456 is not refunded and was taken back", nil
	}
	t.Cleanup(func() { AdtyTranslateFunc = old })

	failed, err := TranslateAdtyDynamic("notice:glossary-rewrite", source, 0)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTranslationFailed, failed.Status)
	assert.Empty(t, failed.English)

	AdtyTranslateFunc = func(_, _ string) (string, error) {
		return "Permanent bonus 12.50 USD order A123456 is not refunded and was clawback", nil
	}
	ready, err := TranslateAdtyDynamic("notice:glossary-kept", source, 0)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTranslationReady, ready.Status)
	assert.True(t, strings.Contains(strings.ToLower(ready.English), "permanent bonus"))
	assert.True(t, strings.Contains(strings.ToLower(ready.English), "clawback"))
}
