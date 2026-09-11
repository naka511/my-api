package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const video933DailyLimitMessage = "Today's model has been restricted, please use it after 8 o'clock tomorrow morning."

// SanitizeVideoTaskFailure maps upstream/internal task failure details to a
// public error that is safe to expose to downstream callers.
func SanitizeVideoTaskFailure(reason string) *dto.OpenAIVideoError {
	return SanitizeVideoTaskFailureForModel("", reason)
}

// SanitizeVideoTaskFailureForModel applies model-specific public error
// mappings before falling back to the generic video error sanitization rules.
func SanitizeVideoTaskFailureForModel(model, reason string) *dto.OpenAIVideoError {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if publicError := Video933DailyLimitError(model, []byte(reason)); publicError != nil {
		return publicError
	}
	code := "upstream_service_error"
	message := "Video generation failed. Please try again later."

	switch {
	case containsAny(normalized,
		"model_not_found",
		"no available channel",
		"under group",
		"distributor",
		"no access to model",
		"this token has no access",
		"unauthorized",
		"forbidden",
		"permission",
		"认证失败",
		"无可用渠道",
		"无权限",
		"模型不存在",
	):
		code = "model_unavailable"
		message = "The requested video model is temporarily unavailable. Please try again later."
	case containsAny(normalized,
		"provider_moderation_error",
		"content_policy",
		"moderation",
		"safety",
		"blocked",
		"内容审核",
		"安全检查",
	):
		code = "content_policy_violation"
		message = "The request was rejected by the safety system. Please modify the prompt and try again."
	case containsAny(normalized,
		"invalid_json",
		"invalid json",
		"json: cannot unmarshal",
		"bad request",
		"invalid_request",
		"invalid parameter",
		"参数",
	):
		code = "invalid_request_error"
		message = "The request parameters are invalid. Please check the request body and try again."
	case containsAny(normalized, "rate limit", "too many requests", "429", "请求过多"):
		code = "rate_limit_exceeded"
		message = "The service is busy. Please try again later."
	}

	return &dto.OpenAIVideoError{
		Code:    code,
		Message: message,
	}
}

// Video933DailyLimitError returns the public error for the 933 daily-limit
// response, or nil when the model/body does not match that exact condition.
func Video933DailyLimitError(model string, body []byte) *dto.OpenAIVideoError {
	if !common.IsVideo933Model(model) || !isVideo933DailyLimitFailureReason(string(body)) {
		return nil
	}
	return &dto.OpenAIVideoError{
		Code:    "model_daily_restriction",
		Message: video933DailyLimitMessage,
	}
}

func isVideo933DailyLimitFailureReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	normalized = strings.ReplaceAll(normalized, `\/`, "/")
	return containsAny(normalized, "risk daily limit", "risk_daily_limit", "risk-daily-limit") &&
		strings.Contains(normalized, "/tools/image-video/generate")
}

func publicVideoErrorType(code string) string {
	switch code {
	case "content_policy_violation", "invalid_request_error":
		return "invalid_request_error"
	default:
		return "server_error"
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func SanitizeOpenAIVideoResponseBody(body []byte) []byte {
	return SanitizeOpenAIVideoResponseBodyForModel("", body)
}

func SanitizeOpenAIVideoResponseBodyForModel(model string, body []byte) []byte {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return body
	}
	rawError, ok := payload["error"]
	if !ok || rawError == nil {
		return body
	}

	errorBytes, err := common.Marshal(rawError)
	if err != nil {
		return body
	}
	if strings.TrimSpace(model) == "" {
		responseModel, _ := payload["model"].(string)
		model = responseModel
	}
	publicError := SanitizeVideoTaskFailureForModel(model, string(errorBytes))
	payload["error"] = map[string]any{
		"code":    publicError.Code,
		"message": publicError.Message,
		"type":    publicVideoErrorType(publicError.Code),
	}

	sanitized, err := common.Marshal(payload)
	if err != nil {
		return body
	}
	return sanitized
}
