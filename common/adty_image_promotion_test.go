package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImagePromotionSkipsAnOlderBuildFinishingLast(t *testing.T) {
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n[ \"${QUERY_FAILED:-0}\" = 0 ] || exit 1\nprintf '%s\\n' \"$MAIN_TIP\"\n"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PROMOTIONS\"\n"), 0700))
	log := filepath.Join(dir, "promotions")
	newSHA, oldSHA := strings.Repeat("b", 40), strings.Repeat("a", 40)
	newDigest, oldDigest := "sha256:"+strings.Repeat("2", 64), "sha256:"+strings.Repeat("1", 64)
	run := func(sha, digest, queryFailed string) error {
		cmd := exec.Command(bash, "../scripts/adty/promote-main-image.sh", digest)
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GITHUB_REPOSITORY=Example/Gateway", "GITHUB_SHA="+sha, "MAIN_TIP="+newSHA,
			"PROMOTIONS="+log, "QUERY_FAILED="+queryFailed)
		return cmd.Run()
	}
	require.NoError(t, run(newSHA, newDigest, "0"))
	require.NoError(t, run(oldSHA, oldDigest, "0"))
	require.Error(t, run(newSHA, newDigest, "1"), "a failed tip lookup must not publish")
	require.Error(t, run(newSHA, "latest", "0"), "promotion must use an immutable digest")
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, "buildx imagetools create --tag ghcr.io/example/gateway:main ghcr.io/example/gateway@"+newDigest+"\n", string(data))
}
