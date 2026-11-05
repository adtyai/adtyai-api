package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const MidjourneyImageAccessTTL = 24 * time.Hour

func midjourneyImageMessage(mjID string, exp int64) []byte {
	return []byte("mj-image\x00" + strings.TrimSpace(mjID) + "\x00" + strconv.FormatInt(exp, 10))
}

// IssueMidjourneyImageAccess signs one image id until exp. The signature is not a user secret.
func IssueMidjourneyImageAccess(mjID string, exp int64) (string, error) {
	mjID = strings.TrimSpace(mjID)
	if mjID == "" || strings.Contains(mjID, "/") || exp <= 0 || common.CryptoSecret == "" {
		return "", ErrTaskArtifactAccessInvalid
	}
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(midjourneyImageMessage(mjID, exp))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// VerifyMidjourneyImageAccess rejects a missing, swapped, or expired signature before any task lookup.
func VerifyMidjourneyImageAccess(mjID, access string, exp, now int64) bool {
	mjID = strings.TrimSpace(mjID)
	if mjID == "" || exp <= now || common.CryptoSecret == "" || len(access) != 43 {
		return false
	}
	actual, err := base64.RawURLEncoding.Strict().DecodeString(access)
	if err != nil || len(actual) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = mac.Write(midjourneyImageMessage(mjID, exp))
	return hmac.Equal(actual, mac.Sum(nil))
}

// MidjourneyImagePath is the signed proxy path, including the expiry query.
func MidjourneyImagePath(mjID string, now time.Time) (string, error) {
	exp := now.Add(MidjourneyImageAccessTTL).Unix()
	access, err := IssueMidjourneyImageAccess(mjID, exp)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("sig", access)
	query.Set("exp", strconv.FormatInt(exp, 10))
	return "/mj/image/" + url.PathEscape(strings.TrimSpace(mjID)) + "?" + query.Encode(), nil
}
