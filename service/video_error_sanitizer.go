package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// SanitizeVideoTaskFailure maps upstream/internal task failure details to a
// public error that is safe to expose to downstream callers.
func SanitizeVideoTaskFailure(reason string) *dto.OpenAIVideoError {
	normalized := strings.ToLower(strings.TrimSpace(reason))
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
	publicError := SanitizeVideoTaskFailure(string(errorBytes))
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
