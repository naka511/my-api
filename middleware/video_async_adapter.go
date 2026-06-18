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
	if model, ok := body["model"].(string); ok {
		switch {
		case strings.EqualFold(model, "sora2"):
			body["model"] = "sora-2"
		case strings.EqualFold(model, "sora2-pro"):
			body["model"] = "sora-2-pro"
		}
	}

	if _, ok := body["seconds"]; !ok {
		if duration, ok := body["duration"]; ok {
			body["seconds"] = normalizeVideoSeconds(duration, "4")
		}
	} else {
		body["seconds"] = normalizeVideoSeconds(body["seconds"], "4")
	}

	if _, ok := body["size"]; !ok {
		if aspectRatio, ok := body["aspect_ratio"].(string); ok {
			switch aspectRatio {
			case "16:9":
				body["size"] = "1280x720"
			case "9:16":
				body["size"] = "720x1280"
			}
		}
	}

	if _, ok := body["input_reference"]; !ok {
		if imageURL, ok := body["image_url"].(string); ok && strings.TrimSpace(imageURL) != "" {
			body["input_reference"] = imageURL
		}
	}
}
