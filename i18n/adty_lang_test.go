package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unofficial leftover codes and regional variants fall back to official
// product languages: Chinese variants to zh-CN, everything else to en.
func TestNormalizeLangOfficialLanguages(t *testing.T) {
	require.NoError(t, Init())
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"zhCN", LangZhCN},
		{"zh-CN", LangZhCN},
		{"zh", LangZhCN},
		{"zh-Hans", LangZhCN},
		{"zhTW", LangZhCN},
		{"zh-TW", LangZhCN},
		{"zh-HK", LangZhCN},
		{"zh-MO", LangZhCN},
		{"zh-Hant", LangZhCN},
		{"en", LangEn},
		{"en-US", LangEn},
		{"fr", LangEn},
		{"fr-FR", LangEn},
		{"fr_CA", LangEn},
		{"ja", LangEn},
		{"ja-JP", LangEn},
		{"ru", LangEn},
		{"ru-RU", LangEn},
		{"vi", LangEn},
		{"vi-VN", LangEn},
		{"ko", LangEn},
		{"ko-KR", LangEn},
		{"de-DE", LangEn},
	} {
		assert.Equal(t, tc.want, normalizeLang(tc.input), tc.input)
	}
	assert.ElementsMatch(t, []string{"zh-CN", "en"}, SupportedLanguages())
	assert.ElementsMatch(t, []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ko", "ru", "vi"}, LoadableLanguages())
	assert.Equal(t, "zhCN", OfficialPreferenceLanguage("zh-TW"))
	assert.Equal(t, "zhCN", OfficialPreferenceLanguage("zh-Hant-HK"))
	assert.Equal(t, "en", OfficialPreferenceLanguage("fr"))
	assert.Equal(t, "en", OfficialPreferenceLanguage("ja-JP"))
	assert.Equal(t, "zhCN", OfficialPreferenceLanguage("zhCN"))
	assert.Equal(t, "en", OfficialPreferenceLanguage("en"))
	assert.Equal(t, Translate(LangZhCN, MsgQuotaUserInsufficient, map[string]any{"Remaining": "$0.10"}),
		Translate("zh-TW", MsgQuotaUserInsufficient, map[string]any{"Remaining": "$0.10"}))
	assert.Equal(t, Translate(LangEn, MsgQuotaUserInsufficient, map[string]any{"Remaining": "$0.10"}),
		Translate("fr", MsgQuotaUserInsufficient, map[string]any{"Remaining": "$0.10"}))
}

// 验证码、找回密码、注册提醒、提现结果、通道维护这几封邮件在 7 种语言下都是译文：
// 不是键名，没有未填的模板变量，除英文外也不是英文原文。
func TestAdtyEmailsAllLanguages(t *testing.T) {
	require.NoError(t, Init())
	params := map[string]any{
		"SystemName": "ADTY AI", "Code": "123456", "Minutes": 10, "Link": "https://example.com/reset",
		"Name": "alipay-1", "Count": 3, "Amount": "12.50", "Txn": "T001", "Reason": "wrong account",
	}
	keys := []string{
		MsgEmailVerificationSubject, MsgEmailVerificationGreeting, MsgEmailVerificationCode, MsgEmailVerificationValidity,
		MsgEmailAccountDeletionSubject, MsgEmailAccountDeletionGreeting, MsgEmailAccountDeletionCode, MsgEmailAccountDeletionValidity,
		MsgEmailPasswordResetSubject, MsgEmailPasswordResetGreeting, MsgEmailPasswordResetLink,
		MsgEmailPasswordResetLinkFallback, MsgEmailPasswordResetValidity,
		MsgAdtyEmailRegisteredSubject, MsgAdtyEmailRegisteredBody,
		MsgAdtySecurityStepUpSubject, MsgAdtySecurityStepUpGreeting, MsgAdtySecurityStepUpCode, MsgAdtySecurityStepUpValidity,
		MsgAdtyReferralWithdrawPaidSubject, MsgAdtyReferralWithdrawPaidBody,
		MsgAdtyReferralWithdrawRejectedSubject, MsgAdtyReferralWithdrawRejectedBody,
		MsgAdtyEpayMaintenanceSubject, MsgAdtyEpayMaintenanceBody,
	}
	for _, lang := range []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ko", "ru", "vi"} {
		for _, key := range keys {
			text := TranslateLoadable(lang, key, params)
			assert.NotEqual(t, key, text, "%s %s", lang, key)
			assert.NotContains(t, text, "{{", "%s %s", lang, key)
			assert.NotContains(t, text, "<no value>", "%s %s", lang, key)
			if lang != LangEn {
				assert.NotEqual(t, Translate(LangEn, key, params), text, "%s %s 还是英文", lang, key)
			}
		}
	}
}
