package service

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSiteLogoAcceptsWebPAndRejectsSVG(t *testing.T) {
	restore := SetAdtySiteAssetDirForTest(t.TempDir())
	t.Cleanup(restore)

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	url, err := SaveAdtySiteLogo(7, png)
	require.NoError(t, err)
	require.Contains(t, url, ".png")

	webp := append([]byte("RIFF\x00\x00\x00\x00WEBP"), bytes.Repeat([]byte{0}, 16)...)
	url, err = SaveAdtySiteLogo(7, webp)
	require.NoError(t, err)
	require.Contains(t, url, ".webp")

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	_, err = SaveAdtySiteLogo(7, svg)
	require.ErrorIs(t, err, ErrAdtySiteLogoType)
}
