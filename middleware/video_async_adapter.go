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
		} else if isWan30Model(modelName) {
			body["seconds"] = defaultSeconds
			body["duration"] = 5
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
		body["size"] = normalizeMiniMaxH3SizeValue(modelName, size)
	}
	normalizeVideo25AsyncOutput(body, modelName)

	if _, ok := body["input_reference"]; !ok {
		if imageURL, ok := body["image_url"].(string); ok && strings.TrimSpace(imageURL) != "" {
			body["input_reference"] = imageURL
		}
	}
}

func isMiniMaxH3Model(model string) bool {
	return common.IsMiniMaxH3Model(model)
}

func isWan30Model(model string) bool {
	return common.IsWan30Model(model)
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
	if isWan30Model(model) {
		modelSizes := map[string]map[string]string{
			"wan3.0-480p":  {"16:9": "854x480", "4:3": "736x552", "1:1": "640x640", "3:4": "552x736", "9:16": "480x854"},
			"wan3.0-720p":  {"16:9": "1280x720", "4:3": "1104x828", "1:1": "960x960", "3:4": "828x1104", "9:16": "720x1280"},
			"wan3.0-1080p": {"16:9": "1920x1080", "4:3": "1656x1242", "1:1": "1440x1440", "3:4": "1242x1656", "9:16": "1080x1920"},
		}
		return modelSizes[strings.ToLower(strings.TrimSpace(model))][strings.TrimSpace(aspectRatio)]
	}
	if isMiniMaxH3Model(model) {
		return miniMaxH3SizeForAspectRatio(model, aspectRatio)
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

func normalizeMiniMaxH3SizeValue(model, size string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	aspectRatio := miniMaxH3AspectRatioFromSize(normalized)
	if aspectRatio == "" {
		return size
	}
	if normalizedSize := miniMaxH3SizeForAspectRatio(model, aspectRatio); normalizedSize != "" {
		return normalizedSize
	}
	return size
}

func miniMaxH3SizeForAspectRatio(model, aspectRatio string) string {
	sizes := map[string]map[string]string{
		"minimax-h3-480p": {"16:9": "856x480", "9:16": "480x856", "1:1": "480x480", "4:3": "640x480", "3:4": "480x640", "21:9": "1120x480"},
		"minimax-h3-768p": {"16:9": "1376x768", "9:16": "768x1376", "1:1": "768x768", "4:3": "1024x768", "3:4": "768x1024", "21:9": "1792x768"},
		"minimax-h3-2k":   {"16:9": "2560x1440", "9:16": "1440x2560", "1:1": "1440x1440", "4:3": "1920x1440", "3:4": "1440x1920", "21:9": "3360x1440"},
		"minimax-h3-4k":   {"16:9": "3840x2160", "9:16": "2160x3840", "1:1": "2160x2160", "4:3": "2880x2160", "3:4": "2160x2880", "21:9": "5040x2160"},
	}
	if strings.TrimSpace(aspectRatio) == "" {
		aspectRatio = "16:9"
	}
	return sizes[strings.ToLower(strings.TrimSpace(model))][strings.TrimSpace(aspectRatio)]
}

func miniMaxH3AspectRatioFromSize(size string) string {
	switch size {
	case "856x480", "1376x768", "2560x1440", "3840x2160", "1280x720", "1920x1080":
		return "16:9"
	case "480x856", "768x1376", "1440x2560", "2160x3840", "720x1280", "1080x1920":
		return "9:16"
	case "480x480", "768x768", "1440x1440", "2160x2160", "1024x1024":
		return "1:1"
	case "640x480", "1024x768", "1920x1440", "2880x2160", "1440x1080":
		return "4:3"
	case "480x640", "768x1024", "1440x1920", "2160x2880", "1080x1440":
		return "3:4"
	case "1120x480", "1792x768", "3360x1440", "5040x2160":
		return "21:9"
	default:
		return ""
	}
}
