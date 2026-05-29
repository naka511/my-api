package sora

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLinkSkyAsyncVideoBodyUsesNumericDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":   "sora-2",
		"seconds": "4",
		"size":    "1280x720",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, "sora2", body["model"])
	require.Equal(t, 4, body["duration"])
	require.Equal(t, "16:9", body["aspect_ratio"])
	require.Equal(t, true, body["async"])
}

func TestNormalizeLinkSkyAsyncVideoBodyCoercesExistingDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":    "video-2.0",
		"duration": "10",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, 10, body["duration"])
}
