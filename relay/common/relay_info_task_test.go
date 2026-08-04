package common

import (
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmitReqUnmarshalMiniMaxH3Fields(t *testing.T) {
	var req TaskSubmitReq
	err := rootcommon.Unmarshal([]byte(`{
		"model":"minimax-h3",
		"prompt":"龟兔赛跑",
		"duration":8,
		"aspect_ratio":"16:9",
		"image_url":"https://example.com/a.png",
		"image_urls":["https://example.com/b.png"],
		"start_image_url":"https://example.com/start.png",
		"end_image_url":"https://example.com/end.png",
		"audio_url":"https://example.com/a.mp3"
	}`), &req)

	require.NoError(t, err)
	require.Equal(t, "minimax-h3", req.Model)
	require.Equal(t, 8, req.Duration)
	require.Equal(t, "16:9", req.AspectRatio)
	require.Equal(t, "https://example.com/a.png", req.ImageURL)
	require.Equal(t, []string{"https://example.com/b.png"}, req.ImageURLs)
	require.Equal(t, "https://example.com/start.png", req.StartImageURL)
	require.Equal(t, "https://example.com/end.png", req.EndImageURL)
	require.Equal(t, "https://example.com/a.mp3", req.AudioURL)
	require.True(t, req.HasImage())
}
