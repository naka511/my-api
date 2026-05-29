package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

func PlaygroundVideoRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost || c.Request.URL.Path != "/pg/chat/completions" {
			c.Next()
			return
		}

		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			c.Next()
			return
		}

		modelName, _ := body["model"].(string)
		if !common.IsOpenAIVideoModel(modelName) {
			c.Next()
			return
		}

		videoBody := buildPlaygroundVideoBody(body)
		jsonData, err := common.Marshal(videoBody)
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

		c.Request.URL.Path = "/v1/video/generations"
		c.Request.Body = io.NopCloser(bytes.NewReader(jsonData))
		c.Request.ContentLength = int64(len(jsonData))
		c.Set("is_playground", true)
		c.Set(common.KeyRequestBody, jsonData)
		c.Set(common.KeyBodyStorage, storage)

		c.Next()
	}
}

func buildPlaygroundVideoBody(body map[string]any) map[string]any {
	videoBody := map[string]any{
		"model":   body["model"],
		"prompt":  extractPlaygroundPrompt(body["messages"]),
		"seconds": "4",
		"size":    "1280x720",
	}
	if group, ok := body["group"]; ok {
		videoBody["group"] = group
	}
	if duration, ok := body["duration"]; ok {
		videoBody["duration"] = duration
	}
	if seconds, ok := body["seconds"]; ok {
		videoBody["seconds"] = seconds
	}
	if size, ok := body["size"]; ok {
		videoBody["size"] = size
	}
	if aspectRatio, ok := body["aspect_ratio"]; ok {
		videoBody["aspect_ratio"] = aspectRatio
	}
	if inputReference, ok := body["input_reference"]; ok {
		videoBody["input_reference"] = inputReference
	}
	if imageURL, ok := body["image_url"]; ok {
		videoBody["image_url"] = imageURL
	}
	return videoBody
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
