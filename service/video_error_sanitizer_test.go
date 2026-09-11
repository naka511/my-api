package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenAIVideoResponseBody(t *testing.T) {
	body := []byte(`{"id":"task_1","status":"failed","error":{"code":"model_not_found","message":"No available channel for model video-2.0 under group zdy (distributor) (request id: abc)","type":"new_api_error"}}`)

	sanitized := SanitizeOpenAIVideoResponseBody(body)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(sanitized, &payload))
	errorPayload, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "model_unavailable", errorPayload["code"])
	require.NotContains(t, string(sanitized), "zdy")
	require.NotContains(t, string(sanitized), "distributor")
	require.NotContains(t, string(sanitized), "request id")
	require.NotContains(t, string(sanitized), "model_not_found")
}

func TestSanitizeVideo933DailyLimit(t *testing.T) {
	publicError := SanitizeVideoTaskFailureForModel(
		"933-video2.0-mini-480p",
		"RISK DAILY LIMIT;endpoint=/tools/image-video/generate",
	)

	require.Equal(t, "model_daily_restriction", publicError.Code)
	require.Equal(t, video933DailyLimitMessage, publicError.Message)
}

func TestSanitizeVideo933DailyLimitDoesNotAffectOtherModels(t *testing.T) {
	publicError := SanitizeVideoTaskFailureForModel(
		"video-2.0",
		"RISK DAILY LIMIT;endpoint=/tools/image-video/generate",
	)

	require.NotEqual(t, "model_daily_restriction", publicError.Code)
	require.NotEqual(t, video933DailyLimitMessage, publicError.Message)
}

func TestSanitizeOpenAIVideoResponseBodyForVideo933DailyLimit(t *testing.T) {
	body := []byte(`{"model":"933-video2.0","status":"failed","error":{"message":"RISK DAILY LIMIT;endpoint=/tools/image-video/generate","type":"server_error"}}`)

	sanitized := SanitizeOpenAIVideoResponseBody(body)

	require.Contains(t, string(sanitized), video933DailyLimitMessage)
	require.NotContains(t, string(sanitized), "RISK DAILY LIMIT")
}
