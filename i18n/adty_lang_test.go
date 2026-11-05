package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 前端保存的 fr、ja、ru、vi 和浏览器的地区写法都落到后端的 4 种新语言。
func TestNormalizeLangNewLanguages(t *testing.T) {
	require.NoError(t, Init())
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"fr", "fr"},
		{"fr-FR", "fr"},
		{"fr_CA", "fr"},
		{"ja", "ja"},
		{"ja-JP", "ja"},
		{"ru", "ru"},
		{"ru-RU", "ru"},
		{"vi", "vi"},
		{"vi-VN", "vi"},
		{"ko", "ko"},
		{"ko-KR", "ko"},
		{"de-DE", LangEn},
	} {
		assert.Equal(t, tc.want, normalizeLang(tc.input), tc.input)
	}
	assert.ElementsMatch(t, []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ko", "ru", "vi"}, SupportedLanguages())
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
		MsgEmailPasswordResetSubject, MsgEmailPasswordResetGreeting, MsgEmailPasswordResetLink,
		MsgEmailPasswordResetLinkFallback, MsgEmailPasswordResetValidity,
		MsgAdtyEmailRegisteredSubject, MsgAdtyEmailRegisteredBody,
		MsgAdtyReferralWithdrawPaidSubject, MsgAdtyReferralWithdrawPaidBody,
		MsgAdtyReferralWithdrawRejectedSubject, MsgAdtyReferralWithdrawRejectedBody,
		MsgAdtyEpayMaintenanceSubject, MsgAdtyEpayMaintenanceBody,
	}
	for _, lang := range []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ko", "ru", "vi"} {
		for _, key := range keys {
			text := Translate(lang, key, params)
			assert.NotEqual(t, key, text, "%s %s", lang, key)
			assert.NotContains(t, text, "{{", "%s %s", lang, key)
			assert.NotContains(t, text, "<no value>", "%s %s", lang, key)
			if lang != LangEn {
				assert.NotEqual(t, Translate(LangEn, key, params), text, "%s %s 还是英文", lang, key)
			}
		}
	}
}
