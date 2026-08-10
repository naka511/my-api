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

func PlaygroundVideoRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			c.Next()
			return
		}

		switch c.Request.URL.Path {
		case "/pg/chat/completions":
			modelName, _ := body["model"].(string)
			if !common.IsOpenAIVideoModel(modelName) {
				c.Next()
				return
			}
			if !replacePlaygroundRequest(c, "/v1/video/generations", buildPlaygroundVideoBody(body)) {
				c.Next()
				return
			}
		case "/pg/images/generations":
			if !replacePlaygroundRequest(c, "/v1/images/generations", buildPlaygroundImageBody(body)) {
				c.Next()
				return
			}
		case "/pg/video/generations":
			if !replacePlaygroundRequest(c, "/v1/video/generations", buildPlaygroundVideoBody(body)) {
				c.Next()
				return
			}
		default:
			c.Next()
			return
		}

		c.Next()
	}
}

func replacePlaygroundRequest(c *gin.Context, path string, body map[string]any) bool {
	jsonData, err := common.Marshal(body)
	if err != nil {
		return false
	}

	if oldStorage, exists := c.Get(common.KeyBodyStorage); exists {
		if storage, ok := oldStorage.(common.BodyStorage); ok {
			_ = storage.Close()
		}
	}
	storage, err := common.CreateBodyStorage(jsonData)
	if err != nil {
		return false
	}

	c.Request.URL.Path = path
	c.Request.Body = io.NopCloser(bytes.NewReader(jsonData))
	c.Request.ContentLength = int64(len(jsonData))
	c.Set("is_playground", true)
	c.Set(common.KeyRequestBody, jsonData)
	c.Set(common.KeyBodyStorage, storage)
	return true
}

func buildPlaygroundImageBody(body map[string]any) map[string]any {
	imageBody := map[string]any{
		"model":  body["model"],
		"prompt": extractPlaygroundBodyPrompt(body),
	}
	if group, ok := body["group"]; ok {
		imageBody["group"] = group
	}
	for _, key := range []string{"n", "size", "quality", "style", "response_format"} {
		if value, ok := body[key]; ok {
			imageBody[key] = value
		}
	}
	return imageBody
}

func buildPlaygroundVideoBody(body map[string]any) map[string]any {
	modelName, _ := body["model"].(string)
	defaultSeconds := "4"
	defaultSize := "1280x720"
	if isMiniMaxH3Model(modelName) {
		defaultSeconds = "5"
		defaultSize = "2560x1440"
	} else if strings.EqualFold(strings.TrimSpace(modelName), "video-2.5-480p") {
		defaultSize = "864x496"
	}
	videoBody := map[string]any{
		"model":  body["model"],
		"prompt": extractPlaygroundBodyPrompt(body),
		"size":   defaultSize,
	}
	if group, ok := body["group"]; ok {
		videoBody["group"] = group
	}
	if duration, ok := body["duration"]; ok {
		videoBody["duration"] = duration
	}
	if seconds, ok := body["seconds"]; ok {
		videoBody["seconds"] = normalizeVideoSeconds(seconds, defaultSeconds)
	} else if duration, ok := videoBody["duration"]; ok {
		videoBody["seconds"] = normalizeVideoSeconds(duration, defaultSeconds)
	} else if _, ok := videoBody["duration"]; !ok {
		videoBody["seconds"] = defaultSeconds
	}
	if size, ok := body["size"]; ok {
		videoBody["size"] = size
	}
	if size, ok := videoBody["size"].(string); ok && isMiniMaxH3Model(modelName) {
		videoBody["size"] = normalizeMiniMaxH3SizeValue(size)
	}
	if _, hasImageURL := videoBody["image_url"]; !hasImageURL {
		if _, hasImageURLs := videoBody["image_urls"]; !hasImageURLs {
			imageURLs := extractPlaygroundImageURLs(body["messages"])
			if len(imageURLs) == 1 {
				videoBody["image_url"] = imageURLs[0]
				videoBody["input_reference"] = imageURLs[0]
			} else if len(imageURLs) > 1 {
				videoBody["image_urls"] = imageURLs
			}
		}
	}
	for _, key := range []string{
		"aspect_ratio",
		"resolution",
		"reference_mode",
		"generate_audio",
		"generateAudio",
		"resolution_name",
		"preset",
		"input_reference",
		"image_url",
		"image_urls",
		"image_reference",
		"images",
		"start_image_url",
		"end_image_url",
		"video_url",
		"video_reference",
		"audio_url",
		"audio_reference",
	} {
		if value, ok := body[key]; ok {
			videoBody[key] = value
		}
	}
	normalizeVideo25AsyncOutput(videoBody, modelName)
	return videoBody
}

func normalizeVideoSeconds(value any, fallback string) string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return v
		}
	case float64:
		if v > 0 {
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	case float32:
		if v > 0 {
			return strconv.FormatFloat(float64(v), 'f', -1, 32)
		}
	case int:
		if v > 0 {
			return strconv.Itoa(v)
		}
	case int64:
		if v > 0 {
			return strconv.FormatInt(v, 10)
		}
	case int32:
		if v > 0 {
			return strconv.FormatInt(int64(v), 10)
		}
	case uint:
		if v > 0 {
			return strconv.FormatUint(uint64(v), 10)
		}
	case uint64:
		if v > 0 {
			return strconv.FormatUint(v, 10)
		}
	case uint32:
		if v > 0 {
			return strconv.FormatUint(uint64(v), 10)
		}
	}
	return fallback
}

func extractPlaygroundImageURLs(messagesValue any) []string {
	messages, ok := messagesValue.([]any)
	if !ok {
		return nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != "user" {
			continue
		}
		content, ok := message["content"].([]any)
		if !ok {
			continue
		}
		var urls []string
		for _, item := range content {
			part, ok := item.(map[string]any)
			if !ok || part["type"] != "image_url" {
				continue
			}
			imageURLValue, ok := part["image_url"]
			if !ok {
				continue
			}
			switch imageURL := imageURLValue.(type) {
			case string:
				if strings.TrimSpace(imageURL) != "" {
					urls = append(urls, strings.TrimSpace(imageURL))
				}
			case map[string]any:
				if url, ok := imageURL["url"].(string); ok && strings.TrimSpace(url) != "" {
					urls = append(urls, strings.TrimSpace(url))
				}
			}
		}
		if len(urls) > 0 {
			return urls
		}
	}
	return nil
}

func extractPlaygroundBodyPrompt(body map[string]any) string {
	if prompt, ok := body["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
		return prompt
	}
	return extractPlaygroundPrompt(body["messages"])
}

func extractPlaygroundPrompt(messagesValue any) string {
	messages, ok := messagesValue.([]any)
	if !ok {
		return "Generate a short cinematic video."
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != "user" {
			continue
		}
		switch content := message["content"].(type) {
		case string:
			if strings.TrimSpace(content) != "" {
				return content
			}
		case []any:
			var parts []string
			for _, item := range content {
				part, ok := item.(map[string]any)
				if !ok || part["type"] != "text" {
					continue
				}
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
	}
	return "Generate a short cinematic video."
}
