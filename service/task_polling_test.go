package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

type capturePollingAdaptor struct {
	body map[string]any
}

func (a *capturePollingAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *capturePollingAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	a.body = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"upstream_task","status":"in_progress","progress":30}`)),
	}, nil
}

func (a *capturePollingAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
}

func (a *capturePollingAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return 0
}

type transientErrorPollingAdaptor struct{}

func (a *transientErrorPollingAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *transientErrorPollingAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary upstream query error","type":"server_error","code":"server_error"}}`)),
	}, nil
}

func (a *transientErrorPollingAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{}, nil
}

func (a *transientErrorPollingAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return 0
}

type failurePollingAdaptor struct {
	reason string
}

func (a *failurePollingAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *failurePollingAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"status":"failed"}`)),
	}, nil
}

func (a *failurePollingAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: a.reason}, nil
}

func (a *failurePollingAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return 0
}

func TestUpdateVideoSingleTaskPassesModelToFetchTask(t *testing.T) {
	truncate(t)

	ch := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "test-channel",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(ch).Error)

	task := &model.Task{
		TaskID:    "public_task",
		ChannelId: 1,
		Platform:  constant.TaskPlatform("1"),
		Status:    model.TaskStatusSubmitted,
		Progress:  "10%",
		Properties: model.Properties{
			UpstreamModelName: "video-2.0-fast",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream_task",
		},
	}
	require.NoError(t, task.Insert())

	adaptor := &capturePollingAdaptor{}
	err := updateVideoSingleTask(context.Background(), adaptor, ch, "upstream_task", map[string]*model.Task{
		"upstream_task": task,
	})

	require.NoError(t, err)
	require.Equal(t, "video-2.0-fast", adaptor.body["model"])
}

func TestUpdateVideoSingleTaskKeepsStatusOnTransientPollError(t *testing.T) {
	truncate(t)

	ch := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "test-channel",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(ch).Error)

	task := &model.Task{
		TaskID:    "public_task",
		ChannelId: 1,
		Platform:  constant.TaskPlatform("1"),
		Status:    model.TaskStatusQueued,
		Progress:  "10%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream_task",
		},
	}
	require.NoError(t, task.Insert())

	err := updateVideoSingleTask(context.Background(), &transientErrorPollingAdaptor{}, ch, "upstream_task", map[string]*model.Task{
		"upstream_task": task,
	})
	require.NoError(t, err)

	var updated model.Task
	require.NoError(t, model.DB.First(&updated, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusQueued), updated.Status)
	require.Equal(t, "10%", updated.Progress)
	require.Empty(t, updated.FailReason)
}

func TestUpdateVideoSingleTaskDelaysRetryableFailure(t *testing.T) {
	truncate(t)

	ch := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "test-channel",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(ch).Error)

	task := &model.Task{
		TaskID:    "public_task",
		ChannelId: 1,
		Platform:  constant.TaskPlatform("1"),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream_task",
		},
	}
	require.NoError(t, task.Insert())

	err := updateVideoSingleTask(context.Background(), &failurePollingAdaptor{reason: "upstream returned error"}, ch, "upstream_task", map[string]*model.Task{
		"upstream_task": task,
	})
	require.NoError(t, err)

	var updated model.Task
	require.NoError(t, model.DB.First(&updated, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), updated.Status)
	require.Equal(t, "30%", updated.Progress)
	require.Empty(t, updated.FailReason)
	require.Equal(t, 1, updated.PrivateData.PendingFailureCount)
	require.Equal(t, "upstream returned error", updated.PrivateData.PendingFailureReason)
}

func TestUpdateVideoSingleTaskConfirmsFailureAfterThreshold(t *testing.T) {
	truncate(t)

	ch := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "test-channel",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(ch).Error)

	task := &model.Task{
		TaskID:    "public_task",
		ChannelId: 1,
		Platform:  constant.TaskPlatform("1"),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            "upstream_task",
			PendingFailureCount:       2,
			PendingFailureFirstSeenAt: 1,
			PendingFailureReason:      "upstream returned error",
		},
	}
	require.NoError(t, task.Insert())

	err := updateVideoSingleTask(context.Background(), &failurePollingAdaptor{reason: "upstream returned error"}, ch, "upstream_task", map[string]*model.Task{
		"upstream_task": task,
	})
	require.NoError(t, err)

	var updated model.Task
	require.NoError(t, model.DB.First(&updated, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), updated.Status)
	require.Equal(t, "100%", updated.Progress)
	require.Equal(t, "upstream returned error", updated.FailReason)
}
