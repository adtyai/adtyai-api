package service

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 收款码和打款凭证图片（任务 6.1）。存在 ./uploads/referral-assets（工作目录是 /data，部署要挂持久卷）；
// 访问不需要登录，靠链接里的过期时间和签名。

const referralAssetLinkTTL = 24 * time.Hour

var (
	errReferralAssetType = errors.New("only PNG, JPEG or WebP images are accepted")
	// referralAssetDir 是存放目录，测试可以换。
	referralAssetDir = func() string { return filepath.Join("uploads", "referral-assets") }
)

var referralAssetExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

// SaveReferralAsset 存一张图片，返回带签名的访问链接。
func SaveReferralAsset(data []byte, contentType string, ownerUserId int, purpose, createdBy string) (string, error) {
	ext, ok := referralAssetExt[contentType]
	if !ok {
		return "", errReferralAssetType
	}
	dir := referralAssetDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d-%s%s", strings.ReplaceAll(purpose, "_", "-"), ownerUserId, common.GetUUID(), ext)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o640); err != nil {
		return "", err
	}
	if err := model.DB.Create(&model.ReferralAsset{
		OwnerUserId: ownerUserId, Purpose: purpose, StoragePath: name, ContentType: contentType, Size: int64(len(data)), CreatedBy: createdBy,
	}).Error; err != nil {
		return "", err
	}
	return SignReferralAssetURL(name, time.Now()), nil
}

func referralAssetSignature(name string, expires int64) string {
	return common.HmacSha256(fmt.Sprintf("%s.%d", name, expires), referralSecret())
}

// SignReferralAssetURL 生成 /api/referral-assets/<文件名>?expires=&sig= 的链接。
func SignReferralAssetURL(name string, now time.Time) string {
	expires := now.Add(referralAssetLinkTTL).Unix()
	values := url.Values{}
	values.Set("expires", strconv.FormatInt(expires, 10))
	values.Set("sig", referralAssetSignature(name, expires))
	return "/api/referral-assets/" + name + "?" + values.Encode()
}

// ResignReferralAssetURL 给存下来的链接重新签名（列表里展示时用，旧链接可能已过期）。
func ResignReferralAssetURL(stored string, now time.Time) string {
	stored = strings.TrimSpace(stored)
	if !strings.HasPrefix(stored, "/api/referral-assets/") {
		return stored
	}
	name := strings.TrimPrefix(stored, "/api/referral-assets/")
	name, _, _ = strings.Cut(name, "?")
	return SignReferralAssetURL(name, now)
}

// OpenReferralAsset 验签后返回文件在磁盘上的路径。
func OpenReferralAsset(name, expiresRaw, sig string, now time.Time) (string, error) {
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", os.ErrNotExist
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil || expires < now.Unix() {
		return "", os.ErrPermission
	}
	if !hmac.Equal([]byte(referralAssetSignature(name, expires)), []byte(sig)) {
		return "", os.ErrPermission
	}
	path := filepath.Join(referralAssetDir(), name)
	if _, err := os.Stat(path); err != nil {
		return "", os.ErrNotExist
	}
	return path, nil
}
