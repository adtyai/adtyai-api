package service

import (
	"crypto/subtle"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
)

// EpaySign 和 go-epay v0.0.5 一致：去掉 sign、sign_type 和空值，按键排序拼 k=v&…，接密钥取 md5。
// 参数有歧义（键为空或含 &、=，值含 &）时报错，不出签名（任务 5.1）。
func EpaySign(params map[string]string, key string) (string, error) {
	signed, err := epay.GenerateParamsWithError(params, key)
	if err != nil {
		return "", err
	}
	return signed["sign"], nil
}

// EpayVerify 用这个通道的密钥验签，常量时间比较；参数有歧义时判失败。
func EpayVerify(params map[string]string, key string) bool {
	got := strings.ToLower(strings.TrimSpace(params["sign"]))
	if got == "" || key == "" {
		return false
	}
	want, err := EpaySign(params, key)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
