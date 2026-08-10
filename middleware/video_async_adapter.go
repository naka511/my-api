package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
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
		} else if isVideo25Model(modelName) {
			body["seconds"] = defaultSeconds
			body["duration"] = 4
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
	normalizeVideo25AsyncOutput(body, modelName)

	if _, ok := body["input_reference"]; !ok {
		if imageURL, ok := body["image_url"].(string); ok && strings.TrimSpace(imageURL) != "" {
			body["input_reference"] = imageURL
		}
	}
}

func isMiniMaxH3Model(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "minimax-h3")
}

func isVideo25Model(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "video-2.5", "video-2.5-480p":
		return true
	default:
		return false
	}
}

func normalizeVideo25AsyncOutput(body map[string]any, model string) {
	if !isVideo25Model(model) {
		return
	}
	if strings.EqualFold(strings.TrimSpace(model), "video-2.5") {
		if _, hasResolution := body["resolution"]; !hasResolution {
			if _, hasSize := body["size"]; !hasSize {
				body["resolution"] = "720p"
			}
		}
		return
	}

	aspectRatio, _ := body["aspect_ratio"].(string)
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" {
		if size, ok := body["size"].(string); ok {
			aspectRatio = video25AspectRatioFromSize(size)
		}
	}
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}
	body["aspect_ratio"] = aspectRatio
	body["resolution"] = "480p"
	switch aspectRatio {
	case "9:16":
		body["size"] = "496x864"
	case "1:1":
		body["size"] = "640x640"
	default:
		body["size"] = "864x496"
	}
}

func video25AspectRatioFromSize(size string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return ""
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return ""
	}
	if width == height {
		return "1:1"
	}
	if width > height {
		return "16:9"
	}
	return "9:16"
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
