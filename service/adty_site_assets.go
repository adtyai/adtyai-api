package service

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// 站长上传的 Logo（任务 11.3）：只收 PNG、JPEG、WebP，不收 SVG。最大 200 KB，存在 ./uploads/site-assets（工作目录是 /data，
// 跟着 /data 一起备份）。地址是 /uploads/site-assets/<随机名>，写进 Brand.logo_url 由站长在 PUT /self 保存。

// AdtySiteLogoMaxBytes 是 Logo 的大小上限。
const AdtySiteLogoMaxBytes = 200 << 10

var (
	ErrAdtySiteLogoTooLarge = common.NewMessage("The logo must be at most 200 KB")
	ErrAdtySiteLogoType     = common.NewMessage("The logo must be a PNG, JPEG, or WebP image")
)

// adtySiteAssetDir 是存放目录，测试可以换。
var adtySiteAssetDir = func() string { return filepath.Join("uploads", "site-assets") }

// adtySiteAssetName 是存储文件名的格式：site-<站点 id>-<uuid>.<扩展名>。
var adtySiteAssetName = regexp.MustCompile(`^site-[0-9]+-[0-9a-f]{32}\.(png|jpg|webp|svg)$`)

// adtySiteLogoKind 判断文件类型：只收 PNG、JPEG 和 WebP。不收 SVG。
func adtySiteLogoKind(data []byte) (ext, contentType string, err error) {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png", "image/png", nil
	case "image/jpeg":
		return ".jpg", "image/jpeg", nil
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return ".webp", "image/webp", nil
	}
	return "", "", ErrAdtySiteLogoType
}

// SaveAdtySiteLogo 存一个 Logo，返回地址（/uploads/site-assets/<名字>）。不改站点：站长在 PUT /self 里保存地址。
func SaveAdtySiteLogo(siteId int, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrAdtySiteLogoType
	}
	if len(data) > AdtySiteLogoMaxBytes {
		return "", ErrAdtySiteLogoTooLarge
	}
	ext, _, err := adtySiteLogoKind(data)
	if err != nil {
		return "", err
	}
	dir := adtySiteAssetDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := "site-" + strconv.Itoa(siteId) + "-" + strings.ReplaceAll(common.GetUUID(), "-", "") + ext
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o640); err != nil {
		return "", err
	}
	return AdtySiteAssetPrefix + name, nil
}

// OpenAdtySiteAsset 按文件名找到 Logo，返回路径和 Content-Type；名字不合格式或文件不在时返回 os.ErrNotExist。
func OpenAdtySiteAsset(name string) (string, string, error) {
	if !adtySiteAssetName.MatchString(name) {
		return "", "", os.ErrNotExist
	}
	path := filepath.Join(adtySiteAssetDir(), name)
	if _, err := os.Stat(path); err != nil {
		return "", "", os.ErrNotExist
	}
	contentType := "image/png"
	switch filepath.Ext(name) {
	case ".jpg":
		contentType = "image/jpeg"
	case ".svg":
		contentType = "image/svg+xml"
	case ".webp":
		contentType = "image/webp"
	}
	return path, contentType, nil
}

// SetAdtySiteAssetDirForTest 换 Logo 的存放目录，返回恢复函数（测试用）。
func SetAdtySiteAssetDirForTest(dir string) func() {
	old := adtySiteAssetDir
	adtySiteAssetDir = func() string { return dir }
	return func() { adtySiteAssetDir = old }
}
