package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAsyncVideoRequestForSora2(t *testing.T) {
	body := map[string]any{
		"model":        "sora2",
		"prompt":       "test",
		"duration":     float64(4),
		"aspect_ratio": "16:9",
		"async":        true,
		"image_url":    "https://example.com/reference.png",
	}

	normalizeAsyncVideoRequest(body)

	require.Equal(t, "sora-2", body["model"])
	require.Equal(t, float64(4), body["seconds"])
	require.Equal(t, "1280x720", body["size"])
	require.Equal(t, "https://example.com/reference.png", body["input_reference"])
}

func TestVideoAsyncRequestConvertRewritesPathAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/v1/video/async-generations", VideoAsyncRequestConvert(), func(c *gin.Context) {
		require.Equal(t, "/v1/video/generations", c.Request.URL.Path)

		rawBody, exists := c.Get(common.KeyRequestBody)
		require.True(t, exists)

		var body map[string]any
		require.NoError(t, common.Unmarshal(rawBody.([]byte), &body))
		require.Equal(t, "sora-2", body["model"])
		require.Equal(t, float64(10), body["seconds"])
		require.Equal(t, "720x1280", body["size"])

		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/video/async-generations", http.NoBody)
	request.Header.Set("Content-Type", "application/json")
	request.Body = io.NopCloser(strings.NewReader(`{"model":"sora2","duration":10,"aspect_ratio":"9:16"}`))

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
