package service

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实的 WebP 样本（40×30），由 PIL 生成后 base64 编进来：
// 这样测试既不需要 cgo，也不依赖任何编码器。
const (
	adtyTestWebPWithEXIF = "UklGRpIAAABXRUJQVlA4WAoAAAAIAAAAJwAAHQAAVlA4IEAAAAAwAwCdASooAB4APm02l0ikIyIh" +
		"JWgAgA2JZwB2AABX74bgAP7vKUf/rUyMfWQ///YO//Qd/+g7/ZTfDcBAAAAARVhJRiwAAABNTQAq" +
		"AAAACAABAQ8AAgAAABIAAAAaAAAAAFNFQ1JFVENBTUVSQU1PREVMAA=="
	adtyTestWebPClean = "UklGRk4AAABXRUJQVlA4IEIAAACQAwCdASooAB4APm02l0ikIqIhJWgAgA2JZwDQ+oB+AAAr98Nw" +
		"AP7tBy//npmvMH9eH/9GH8MP4Yf0bn5H0gAAAAA="
)

func adtyTestDefaultImagePolicy() model.AdtyTicketPolicy {
	return model.AdtyTicketPolicy{MaxSide: 4096, MaxPixels: 16_000_000, MaxTotal: 10 << 20}
}

// ISS-R4-0020：WebP 以前只按 RIFF/WEBP 魔数判断，然后原样返回原始字节，
// 既不重新编码、也不真正清理元数据（只做了 EXIF/XMP 字样扫描，换块名就绕过）。
// 现在统一解码后存成 PNG：元数据随重编码消失，内容也被真正解析过。
func TestAdtyTicketImageTranscodesWebPToPNG(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(adtyTestWebPWithEXIF)
	require.NoError(t, err)

	stored, mime, width, height, err := sanitizeAdtyTicketImage(raw, adtyTestDefaultImagePolicy())
	require.NoError(t, err)

	assert.Equal(t, "image/png", mime, "WebP 必须重编码成 PNG 落库")
	assert.Equal(t, 40, width)
	assert.Equal(t, 30, height)
	assert.True(t, bytes.HasPrefix(stored, []byte("\x89PNG\r\n\x1a\n")), "落库内容必须是真正的 PNG")
	assert.NotEqual(t, raw, stored, "不能原样返回上传字节")
	assert.NotContains(t, string(stored), "SECRETCAMERAMODEL", "EXIF 元数据必须被重编码清掉")
	assert.NotContains(t, string(stored), "EXIF")
	assert.NotContains(t, string(stored), "XMP")
}

// 不带元数据的 WebP 同样要能上传，只是同样被转成 PNG。
func TestAdtyTicketImageAcceptsCleanWebP(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(adtyTestWebPClean)
	require.NoError(t, err)

	stored, mime, _, _, err := sanitizeAdtyTicketImage(raw, adtyTestDefaultImagePolicy())
	require.NoError(t, err)
	assert.Equal(t, "image/png", mime)
	assert.True(t, bytes.HasPrefix(stored, []byte("\x89PNG\r\n\x1a\n")))
}

// 伪装文件不能借 WebP 分支蒙混过关：只有 RIFF 头、里面不是有效 WebP。
func TestAdtyTicketImageRejectsFakeWebP(t *testing.T) {
	fake := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0x00}, 32)...)
	_, _, _, _, err := sanitizeAdtyTicketImage(fake, adtyTestDefaultImagePolicy())
	require.Error(t, err, "魔数对但解码不了的文件必须拒绝")
}

// 超出尺寸上限的 WebP 仍然要被挡住——转码不能放宽策略。
func TestAdtyTicketImageRejectsOversizedWebP(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(adtyTestWebPClean)
	require.NoError(t, err)
	policy := adtyTestDefaultImagePolicy()
	policy.MaxSide = 20
	_, _, _, _, err = sanitizeAdtyTicketImage(raw, policy)
	require.Error(t, err)
}
