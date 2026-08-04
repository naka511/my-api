package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

func VideoAsyncRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("local_async_video_submit", true)
		c.Request.URL.Path = strings.Replace(c.Request.URL.Path, "/v1/video/async-generations", "/v1/video/generations", 1)

		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			c.Next()
			return
		}

		normalizeAsyncVideoRequest(body)

		jsonData, err := common.Marshal(body)
		if err != nil {
			c.Next()
			return
		}

		if oldStorage, exists := c.Get(common.KeyBodyStorage); exists {
			if storage, ok := oldStorage.(common.BodyStorage); ok {
				_ = storage.Close()
			}
		}
		storage, err := common.CreateBodyStorage(jsonData)
		if err != nil {
			c.Next()
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(jsonData))
		c.Request.ContentLength = int64(len(jsonData))
		c.Set(common.KeyRequestBody, jsonData)
		c.Set(common.KeyBodyStorage, storage)

		c.Next()
	}
}

func normalizeAsyncVideoRequest(body map[string]any) {
	modelName, _ := body["model"].(string)
	if model, ok := body["model"].(string); ok {
		switch {
		case strings.EqualFold(model, "sora2"):
			body["model"] = "sora-2"
		case strings.EqualFold(model, "sora2-pro"):
			body["model"] = "sora-2-pro"
		}
	}

	defaultSeconds := "4"
	if isMiniMaxH3Model(modelName) {
		defaultSeconds = "5"
	}
	if _, ok := body["seconds"]; !ok {
		if duration, ok := body["duration"]; ok {
			body["seconds"] = normalizeVideoSeconds(duration, defaultSeconds)
		}
	} else {
		body["seconds"] = normalizeVideoSeconds(body["seconds"], defaultSeconds)
	}

	if _, ok := body["size"]; !ok {
		if aspectRatio, ok := body["aspect_ratio"].(string); ok {
			if size := defaultVideoSizeForAspectRatio(modelName, aspectRatio); size != "" {
				body["size"] = size
			}
		}
	}
	if size, ok := body["size"].(string); ok && isMiniMaxH3Model(modelName) {
		body["size"] = normalizeMiniMaxH3SizeValue(size)
	}

	if _, ok := body["input_reference"]; !ok {
		if imageURL, ok := body["image_url"].(string); ok && strings.TrimSpace(imageURL) != "" {
			body["input_reference"] = imageURL
		}
	}
}

func isMiniMaxH3Model(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "minimax-h3")
}

func defaultVideoSizeForAspectRatio(model string, aspectRatio string) string {
	if isMiniMaxH3Model(model) {
		switch strings.TrimSpace(aspectRatio) {
		case "9:16":
			return "1440x2560"
		case "1:1":
			return "1440x1440"
		case "4:3":
			return "1920x1440"
		case "3:4":
			return "1440x1920"
		case "21:9":
			return "3360x1440"
		default:
			return "2560x1440"
		}
	}

	switch strings.TrimSpace(aspectRatio) {
	case "16:9":
		return "1280x720"
	case "9:16":
		return "720x1280"
	default:
		return ""
	}
}

func normalizeMiniMaxH3SizeValue(size string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x"))) {
	case "1280x720", "1920x1080", "2560x1440":
		return "2560x1440"
	case "720x1280", "1080x1920", "1440x2560":
		return "1440x2560"
	case "1024x1024", "1440x1440":
		return "1440x1440"
	case "1440x1080", "1920x1440":
		return "1920x1440"
	case "1080x1440", "1440x1920":
		return "1440x1920"
	case "3360x1440":
		return "3360x1440"
	default:
		return size
	}
}
