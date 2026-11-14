package i18n

import (
	"strings"
	"testing"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/QuantumNous/new-api/common"
)

func addTestMessage(t *testing.T, lang, id, other string) {
	t.Helper()
	tag, err := language.Parse(lang)
	require.NoError(t, err)
	require.NoError(t, bundle.AddMessages(tag, &goi18n.Message{ID: id, Other: other}))
}

func assertSafeFallback(t *testing.T, lang, key, text string) {
	t.Helper()
	assert.NotEmpty(t, text)
	assert.NotEqual(t, key, text)
	assert.NotContains(t, text, key)
	assert.NotContains(t, text, "{{")
	lower := strings.ToLower(text)
	assert.NotContains(t, lower, "messageid")
	assert.NotContains(t, lower, "template:")
	assert.NotContains(t, lower, "panic")
	assert.NotContains(t, text, "\n")
}

func TestTranslateFallback(t *testing.T) {
	require.NoError(t, Init())
	t.Cleanup(func() { common.DefaultLanguage = "" })

	t.Run("successful translations are unchanged", func(t *testing.T) {
		quota := map[string]any{"Remaining": "$0.10"}
		assert.Equal(t, "Insufficient user quota, remaining quota: $0.10",
			Translate(LangEn, MsgQuotaUserInsufficient, quota))
		assert.Equal(t, "用户额度不足, 剩余额度: $0.10",
			Translate(LangZhCN, MsgQuotaUserInsufficient, quota))
	})

	t.Run("missing key does not return the raw key", func(t *testing.T) {
		const key = "test.iss_r1_0053.missing_in_both_official_packs"
		zh := Translate(LangZhCN, key)
		en := Translate(LangEn, key)
		assertSafeFallback(t, LangZhCN, key, zh)
		assertSafeFallback(t, LangEn, key, en)
		assert.Equal(t, fallbackTranslationZhCN, zh)
		assert.Equal(t, fallbackTranslationEn, en)
		assert.NotEqual(t, zh, en)
	})

	t.Run("official zh-CN and en each get a readable fallback", func(t *testing.T) {
		const key = "test.iss_r1_0053.generic_fallback"
		assert.Equal(t, fallbackTranslationZhCN, Translate(LangZhCN, key))
		assert.Equal(t, fallbackTranslationEn, Translate(LangEn, key))
		assert.True(t, strings.ContainsAny(Translate(LangZhCN, key), "暂无法显示提示稍后重试"))
		assert.True(t, strings.Contains(Translate(LangEn, key), "could not be displayed"))
	})

	t.Run("unofficial leftover codes are coerced and do not leak keys", func(t *testing.T) {
		const key = "test.iss_r1_0053.leftover_coerced"
		for _, lang := range []string{"zhTW", "zh-TW", "zh-HK", "zh-Hant"} {
			text := Translate(lang, key)
			assertSafeFallback(t, lang, key, text)
			assert.Equal(t, fallbackTranslationZhCN, text, lang)
		}
		for _, lang := range []string{"fr", "ja", "ru", "vi", "ko", "de"} {
			text := Translate(lang, key)
			assertSafeFallback(t, lang, key, text)
			assert.Equal(t, fallbackTranslationEn, text, lang)
		}
	})

	t.Run("missing in the requested pack falls back to the other official language", func(t *testing.T) {
		const key = "test.iss_r1_0053.en_only"
		addTestMessage(t, LangEn, key, "English-only fallback source")
		assert.Equal(t, "English-only fallback source", Translate(LangEn, key))
		assert.Equal(t, "English-only fallback source", Translate(LangZhCN, key))
		assert.NotEqual(t, key, Translate(LangZhCN, key))
	})

	t.Run("failed template does not return the raw key or internal error text", func(t *testing.T) {
		const bothBroken = "test.iss_r1_0053.broken_template_both"
		addTestMessage(t, LangEn, bothBroken, "Hello {{.Name")
		addTestMessage(t, LangZhCN, bothBroken, "你好 {{.Name")
		zh := Translate(LangZhCN, bothBroken, map[string]any{"Name": "Ada"})
		en := Translate(LangEn, bothBroken, map[string]any{"Name": "Ada"})
		assertSafeFallback(t, LangZhCN, bothBroken, zh)
		assertSafeFallback(t, LangEn, bothBroken, en)
		assert.Equal(t, fallbackTranslationZhCN, zh)
		assert.Equal(t, fallbackTranslationEn, en)

		const zhBroken = "test.iss_r1_0053.broken_template_zh"
		addTestMessage(t, LangZhCN, zhBroken, "你好 {{.Name")
		addTestMessage(t, LangEn, zhBroken, "Hello {{.Name}}")
		assert.Equal(t, "Hello Ada", Translate(LangZhCN, zhBroken, map[string]any{"Name": "Ada"}))
		assert.Equal(t, "Hello Ada", Translate(LangEn, zhBroken, map[string]any{"Name": "Ada"}))
		assert.NotContains(t, Translate(LangZhCN, zhBroken, map[string]any{"Name": "Ada"}), "template")
	})

	t.Run("empty language follows the official default and still does not leak keys", func(t *testing.T) {
		const key = "test.iss_r1_0053.empty_lang"
		common.DefaultLanguage = ""
		assert.Equal(t, fallbackTranslationEn, Translate("", key))
		common.DefaultLanguage = LangZhCN
		assert.Equal(t, fallbackTranslationZhCN, Translate("", key))
		common.DefaultLanguage = LangEn
		assert.Equal(t, fallbackTranslationEn, Translate("", key))
	})
}
