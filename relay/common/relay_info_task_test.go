package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
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

func TestValidateBasicTaskRequestMultipartMiniMaxH3Fields(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "minimax-h3"))
	require.NoError(t, writer.WriteField("prompt", "龟兔赛跑"))
	require.NoError(t, writer.WriteField("duration", "8"))
	require.NoError(t, writer.WriteField("aspect_ratio", "16:9"))
	require.NoError(t, writer.WriteField("image_url", "https://example.com/a.png"))
	require.NoError(t, writer.WriteField("image_urls", "https://example.com/b.png"))
	require.NoError(t, writer.WriteField("image_urls", "https://example.com/c.png"))
	require.NoError(t, writer.WriteField("audio_url", "https://example.com/a.mp3"))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = request

	taskErr := ValidateBasicTaskRequest(c, &RelayInfo{}, constant.TaskActionGenerate)
	require.Nil(t, taskErr)

	req, err := GetTaskRequest(c)
	require.NoError(t, err)
	require.Equal(t, "minimax-h3", req.Model)
	require.Equal(t, "龟兔赛跑", req.Prompt)
	require.Equal(t, 8, req.Duration)
	require.Equal(t, "16:9", req.AspectRatio)
	require.Equal(t, "https://example.com/a.mp3", req.AudioURL)
	require.Equal(t, []string{
		"https://example.com/a.png",
		"https://example.com/b.png",
		"https://example.com/c.png",
	}, req.Images)
}
