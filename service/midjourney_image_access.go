package service

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

const midjourneyImageArtifactKey = "mj-image"

func IssueMidjourneyImageAccess(mjId string) (string, error) {
	return IssueTaskArtifactAccess(strings.TrimSpace(mjId), midjourneyImageArtifactKey)
}

func VerifyMidjourneyImageAccess(mjId, access string) bool {
	return VerifyTaskArtifactAccess(access, strings.TrimSpace(mjId), midjourneyImageArtifactKey)
}

// MidjourneyForwardImageURL builds a browser-loadable proxy URL. An empty
// result means the capability could not be signed; callers must not publish
// an unsigned /mj/image path.
func MidjourneyForwardImageURL(serverAddress, mjId string, cacheBust bool) string {
	mjId = strings.TrimSpace(mjId)
	access, err := IssueMidjourneyImageAccess(mjId)
	if err != nil || mjId == "" {
		return ""
	}
	imageURL := strings.TrimRight(strings.TrimSpace(serverAddress), "/") + "/mj/image/" + mjId + "?" + TaskArtifactAccessQueryParameter + "=" + url.QueryEscape(access)
	if cacheBust {
		imageURL += "&rand=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return imageURL
}
