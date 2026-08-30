package common

import "strings"

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	ImageGenerationModels = []string{
		"dall-e-3",
		"dall-e-2",
		"gpt-image-1",
		"prefix:imagen-",
		"flux",
		"midjourney",
		"nano-banana",
		"gpt-image2",
	}
	OpenAIVideoModels = []string{
		"video-2.0",
		"video-2.0-fast",
		"video-2.5",
		"video-2.5-480p",
		"sora-2",
		"sora2",
		"veo31",
		"veo31-fast",
		"veo31-ref",
		"kling-v3",
		"grok-imagine-video",
		"ko3",
		"minimax-h3-480p",
		"minimax-h3-768p",
		"minimax-h3-2k",
		"minimax-h3-4k",
		"wan3.0-480p",
		"wan3.0-720p",
		"wan3.0-1080p",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

var MiniMaxH3Models = []string{
	"minimax-h3-480p",
	"minimax-h3-768p",
	"minimax-h3-2k",
	"minimax-h3-4k",
}

func IsMiniMaxH3Model(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	for _, model := range MiniMaxH3Models {
		if normalized == model {
			return true
		}
	}
	return false
}

var Wan30Models = []string{
	"wan3.0-480p",
	"wan3.0-720p",
	"wan3.0-1080p",
}

func IsWan30Model(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	for _, model := range Wan30Models {
		if normalized == model {
			return true
		}
	}
	return false
}

func IsImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if strings.Contains(modelName, m) {
			return true
		}
		if strings.HasPrefix(m, "prefix:") && strings.HasPrefix(modelName, strings.TrimPrefix(m, "prefix:")) {
			return true
		}
	}
	return false
}

func IsOpenAIVideoModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAIVideoModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
