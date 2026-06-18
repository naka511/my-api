package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestNormalizeLinkSkyAsyncVideoBodyUsesNumericDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":   "sora-2",
		"seconds": "4",
		"size":    "1280x720",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, "sora2", body["model"])
	require.Equal(t, 4, body["duration"])
	require.Equal(t, "16:9", body["aspect_ratio"])
	require.Equal(t, true, body["async"])
}

func TestNormalizeLinkSkyAsyncVideoBodyCoercesExistingDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":    "video-2.0",
		"duration": "10",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, 10, body["duration"])
}

func TestBuildRequestURLUsesAsyncEndpointForLinkSkyOpenAIChannel(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://linksky.top",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://linksky.top/v1/video/async-generations", url)
}

func TestBuildRequestURLKeepsOpenAIVideoEndpointForOtherOpenAIChannels(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://api.openai.com",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/videos", url)
}

func TestBuildRequestURLUsesAsyncEndpointForLeoGoModelAlias(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://example-leogo.test",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "video-2.0-fast"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://example-leogo.test/v1/video/async-generations", url)
}

func TestResponseTaskUpstreamTaskIDFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		body     responseTask
		expected string
	}{
		{
			name:     "uses request id",
			body:     responseTask{RequestID: "req_123"},
			expected: "req_123",
		},
		{
			name:     "uses poll url last segment",
			body:     responseTask{PollURL: "/v1/video/async-generations/task_456?foo=bar"},
			expected: "task_456",
		},
		{
			name:     "uses nested data task id",
			body:     responseTask{Data: []byte(`{"task_id":"nested_789"}`)},
			expected: "nested_789",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.body.upstreamTaskID())
		})
	}
}

func TestUpstreamTaskIDFromResponseBodySupportsWrappedShapes(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "uses nested data id",
			body:     `{"code":0,"message":"ok","data":{"id":"data_id_123","status":"in_progress"}}`,
			expected: "data_id_123",
		},
		{
			name:     "uses nested data task id array",
			body:     `{"code":0,"data":[{"task_id":"array_task_456"}]}`,
			expected: "array_task_456",
		},
		{
			name:     "uses data string",
			body:     `{"code":0,"data":"string_task_789"}`,
			expected: "string_task_789",
		},
		{
			name:     "uses generation id alias",
			body:     `{"code":0,"result":{"generation_id":"generation_abc"}}`,
			expected: "generation_abc",
		},
		{
			name:     "uses job id alias",
			body:     `{"code":0,"task":{"job_id":"job_def"}}`,
			expected: "job_def",
		},
		{
			name:     "uses nested poll url",
			body:     `{"code":0,"data":{"poll_url":"/v1/video/async-generations/poll_task_321"}}`,
			expected: "poll_task_321",
		},
		{
			name:     "does not use arbitrary message string",
			body:     `{"code":"success","message":"ok","data":null}`,
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, upstreamTaskIDFromResponseBody([]byte(test.body)))
		})
	}
}

func TestFetchTaskUsesAsyncEndpointForModelAlias(t *testing.T) {
	service.InitHttpClient()

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = io.WriteString(w, `{"task_id":"task_123","status":"in_progress"}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{ChannelType: constant.ChannelTypeOpenAI}
	resp, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{
		"task_id": "task_123",
		"model":   "video-2.0-fast",
	}, "")
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	require.Equal(t, "/v1/video/async-generations/task_123", requestedPath)
}

func TestParseTaskResultReadsDocumentResultURLFallbacks(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		expectedURL string
	}{
		{
			name:        "uses video url first",
			body:        `{"task_id":"task_1","status":"completed","progress":100,"video_url":"https://example.com/video.mp4","url":"https://example.com/fallback.mp4"}`,
			expectedURL: "https://example.com/video.mp4",
		},
		{
			name:        "uses result url",
			body:        `{"task_id":"task_1","status":"completed","progress":100,"result_url":"https://example.com/result.png"}`,
			expectedURL: "https://example.com/result.png",
		},
		{
			name:        "uses data item url",
			body:        `{"task_id":"task_1","status":"completed","progress":100,"data":[{"url":"https://example.com/data.png"}]}`,
			expectedURL: "https://example.com/data.png",
		},
		{
			name:        "uses wrapped data video url",
			body:        `{"code":"success","data":{"task_id":"task_1","status":"completed","progress":100,"video_url":"https://example.com/wrapped.mp4"}}`,
			expectedURL: "https://example.com/wrapped.mp4",
		},
		{
			name:        "uses uppercase task status and output video url",
			body:        `{"code":"success","data":{"task_id":"task_1","task_status":"SUCCESS","output":{"video_url":"https://example.com/output.mp4"}}}`,
			expectedURL: "https://example.com/output.mp4",
		},
		{
			name:        "infers completed when result url exists without status",
			body:        `{"code":"success","data":{"task_id":"task_1","output":{"url":"https://example.com/no-status.mp4"}}}`,
			expectedURL: "https://example.com/no-status.mp4",
		},
	}

	adaptor := &TaskAdaptor{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskInfo, err := adaptor.ParseTaskResult([]byte(test.body))

			require.NoError(t, err)
			require.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
			require.Equal(t, test.expectedURL, taskInfo.Url)
		})
	}
}

func TestParseTaskResultKeepsSuccessLikeUnknownResponseInProgress(t *testing.T) {
	adaptor := &TaskAdaptor{}

	taskInfo, err := adaptor.ParseTaskResult([]byte(`{"code":"success","message":"ok","data":null}`))

	require.NoError(t, err)
	require.Equal(t, model.TaskStatusInProgress, taskInfo.Status)
}

func TestNormalizeTaskStatusAliases(t *testing.T) {
	tests := map[string]string{
		"SUCCESS":     "completed",
		"SUCCEEDED":   "completed",
		"COMPLETED":   "completed",
		"IN_PROGRESS": "in_progress",
		"RUNNING":     "in_progress",
		"FAILURE":     "failed",
		"CANCELLED":   "failed",
		"PENDING":     "queued",
	}

	for input, expected := range tests {
		require.Equal(t, expected, normalizeTaskStatus(input))
	}
}
