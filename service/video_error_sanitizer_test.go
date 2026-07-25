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
