package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundVideoRequestConvert(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		body         string
		expectedPath string
		expectedBody map[string]any
	}{
		{
			name:         "video model uses video generations",
			path:         "/pg/video/generations",
			body:         `{"model":"sora2","group":"default","messages":[{"role":"user","content":"make a video"}]}`,
			expectedPath: "/v1/video/generations",
			expectedBody: map[string]any{
				"model":   "sora2",
				"group":   "default",
				"prompt":  "make a video",
				"seconds": "4",
				"size":    "1280x720",
			},
		},
		{
			name:         "image model uses image generations",
			path:         "/pg/images/generations",
			body:         `{"model":"flux","prompt":"custom image prompt","messages":[{"role":"user","content":"make an image"}]}`,
			expectedPath: "/v1/images/generations",
			expectedBody: map[string]any{
				"model":  "flux",
				"prompt": "custom image prompt",
			},
		},
		{
			name:         "legacy chat video request remains compatible",
			path:         "/pg/chat/completions",
			body:         `{"model":"video-2.0-fast","messages":[{"role":"user","content":"legacy video"}]}`,
			expectedPath: "/v1/video/generations",
			expectedBody: map[string]any{
				"model":   "video-2.0-fast",
				"prompt":  "legacy video",
				"seconds": "4",
				"size":    "1280x720",
			},
		},
		{
			name:         "video duration selection is preserved",
			path:         "/pg/video/generations",
			body:         `{"model":"video-2.0-fast","duration":12,"messages":[{"role":"user","content":"longer video"}]}`,
			expectedPath: "/v1/video/generations",
			expectedBody: map[string]any{
				"model":    "video-2.0-fast",
				"prompt":   "longer video",
				"duration": float64(12),
				"seconds":  "12",
				"size":     "1280x720",
			},
		},
		{
			name:         "minimax h3 uses supported 2k size",
			path:         "/pg/video/generations",
			body:         `{"model":"minimax-h3","duration":5,"size":"1280x720","messages":[{"role":"user","content":"h3 video"}]}`,
			expectedPath: "/v1/video/generations",
			expectedBody: map[string]any{
				"model":    "minimax-h3",
				"prompt":   "h3 video",
				"duration": float64(5),
				"seconds":  "5",
				"size":     "2560x1440",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(PlaygroundVideoRequestConvert())
			router.POST("/*path", func(c *gin.Context) {
				var body map[string]any
				require.NoError(t, common.DecodeJson(c.Request.Body, &body))
				require.Equal(t, test.expectedPath, c.Request.URL.Path)
				require.Equal(t, test.expectedBody, body)
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}
