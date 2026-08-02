package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var localAsyncVideoSubmitWorkerOnce sync.Once

type localAsyncVideoSubmitConfig struct {
	Concurrency       int
	BatchSize         int
	IntervalSeconds   int
	StuckResetMinutes int
}

func StartLocalAsyncVideoSubmitWorker() {
	localAsyncVideoSubmitWorkerOnce.Do(func() {
		go localAsyncVideoSubmitLoop()
	})
}

func isLocalQueuedVideoSubmit(c *gin.Context) bool {
	return c != nil &&
		c.Request != nil &&
		c.Request.Method == http.MethodPost &&
		(c.GetBool("local_async_video_submit") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/video/async-generations"))
}

func RelayTaskLocalQueue(c *gin.Context, relayInfo *relaycommon.RelayInfo) bool {
	if !isLocalQueuedVideoSubmit(c) {
		return false
	}
	StartLocalAsyncVideoSubmitWorker()

	bodyStorage, bodyErr := common.GetBodyStorage(c)
	if bodyErr != nil {
		if common.IsRequestBodyTooLargeError(bodyErr) {
			respondTaskError(c, service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge))
		} else {
			respondTaskError(c, service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest))
		}
		return true
	}
	requestBody, bodyErr := bodyStorage.Bytes()
	if bodyErr != nil {
		respondTaskError(c, service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest))
		return true
	}
	_, _ = bodyStorage.Seek(0, 0)
	c.Request.Body = io.NopCloser(bodyStorage)

	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	prepared, taskErr := relay.PrepareTaskSubmit(c, relayInfo)
	if taskErr != nil {
		respondTaskError(c, taskErr)
		return true
	}

	service.LogTaskConsumption(c, relayInfo)

	task := model.InitTask(prepared.Platform, relayInfo)
	task.PrivateData.SubmitRequestBody = requestBody
	task.PrivateData.SubmitContentSummary = buildTaskContentSummary(task, requestBody)
	task.ContentPreview = buildTaskContentPreview(task)
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	task.PrivateData.TokenId = relayInfo.TokenId
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      relayInfo.PriceData.ModelPrice,
		GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      relayInfo.PriceData.ModelRatio,
		OtherRatios:     relayInfo.PriceData.OtherRatios,
		OriginModelName: relayInfo.OriginModelName,
		PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
	}
	task.Quota = prepared.Quota
	task.Action = relayInfo.Action
	task.Status = model.TaskStatusSubmitted
	task.Progress = "0%"
	if insertErr := task.Insert(); insertErr != nil {
		taskErr = service.TaskErrorWrapperLocal(insertErr, "insert_task_failed", http.StatusInternalServerError)
		respondTaskError(c, taskErr)
		return true
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         task.TaskID,
		"task_id":    task.TaskID,
		"request_id": task.TaskID,
		"poll_url":   "/v1/video/async-generations/" + task.TaskID,
		"object":     "video.generation",
		"model":      relayInfo.OriginModelName,
		"status":     dto.VideoStatusQueued,
		"progress":   0,
		"created_at": task.SubmitTime,
	})
	return true
}

func localAsyncVideoSubmitLoop() {
	config := loadLocalAsyncVideoSubmitConfig()
	logger.LogInfo(context.Background(), fmt.Sprintf(
		"local async video submit worker started: concurrency=%d batch=%d interval=%ds stuck_reset=%dm",
		config.Concurrency,
		config.BatchSize,
		config.IntervalSeconds,
		config.StuckResetMinutes,
	))

	sem := make(chan struct{}, config.Concurrency)
	for {
		ctx := context.Background()
		resetStuckLocalQueuedVideoTasks(ctx, config)
		submitLocalQueuedVideoTasks(ctx, config, sem)
		time.Sleep(time.Duration(config.IntervalSeconds) * time.Second)
	}
}

func loadLocalAsyncVideoSubmitConfig() localAsyncVideoSubmitConfig {
	concurrency := common.GetEnvOrDefault("LOCAL_ASYNC_VIDEO_SUBMIT_CONCURRENCY", 20)
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 200 {
		concurrency = 200
	}

	batchSize := common.GetEnvOrDefault("LOCAL_ASYNC_VIDEO_SUBMIT_BATCH_SIZE", concurrency*4)
	if batchSize < concurrency {
		batchSize = concurrency
	}
	if batchSize > 1000 {
		batchSize = 1000
	}

	intervalSeconds := common.GetEnvOrDefault("LOCAL_ASYNC_VIDEO_SUBMIT_INTERVAL_SECONDS", 1)
	if intervalSeconds < 1 {
		intervalSeconds = 1
	}

	stuckResetMinutes := common.GetEnvOrDefault("LOCAL_ASYNC_VIDEO_SUBMIT_STUCK_RESET_MINUTES", 5)
	if stuckResetMinutes < 1 {
		stuckResetMinutes = 1
	}

	return localAsyncVideoSubmitConfig{
		Concurrency:       concurrency,
		BatchSize:         batchSize,
		IntervalSeconds:   intervalSeconds,
		StuckResetMinutes: stuckResetMinutes,
	}
}

func resetStuckLocalQueuedVideoTasks(ctx context.Context, config localAsyncVideoSubmitConfig) {
	cutoff := time.Now().Unix() - int64(config.StuckResetMinutes)*60
	rows, err := model.ResetStuckLocalQueuedSubmitTasks(cutoff, config.BatchSize)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("reset stuck local queued video tasks failed: %s", err.Error()))
		return
	}
	if rows > 0 {
		logger.LogWarn(ctx, fmt.Sprintf("reset %d stuck local queued video submit tasks", rows))
	}
}

func submitLocalQueuedVideoTasks(ctx context.Context, config localAsyncVideoSubmitConfig, sem chan struct{}) {
	available := config.Concurrency - len(sem)
	if available <= 0 {
		return
	}
	limit := config.BatchSize
	if limit > available {
		limit = available
	}

	tasks := model.GetLocalQueuedSubmitTasks(limit)
	if len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		select {
		case sem <- struct{}{}:
		default:
			return
		}
		claimed, err := model.ClaimLocalQueuedSubmitTask(task)
		if err != nil {
			<-sem
			logger.LogError(ctx, fmt.Sprintf("claim local queued video task %s failed: %s", task.TaskID, err.Error()))
			continue
		}
		if !claimed {
			<-sem
			continue
		}
		go func(task *model.Task) {
			defer func() {
				<-sem
			}()
			if err := submitLocalQueuedVideoTask(ctx, task); err != nil {
				logger.LogError(ctx, fmt.Sprintf("submit local queued video task %s failed: %s", task.TaskID, err.Error()))
			}
		}(task)
	}
}

func submitLocalQueuedVideoTask(ctx context.Context, task *model.Task) error {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		failLocalQueuedVideoTask(ctx, task, fmt.Sprintf("get channel failed: %s", err.Error()))
		return err
	}
	if channelModel.Status != common.ChannelStatusEnabled {
		err = fmt.Errorf("channel #%d is disabled", channelModel.Id)
		failLocalQueuedVideoTask(ctx, task, err.Error())
		return err
	}

	c, relayInfo, err := buildLocalQueuedSubmitContext(task, channelModel)
	if err != nil {
		failLocalQueuedVideoTask(ctx, task, err.Error())
		return err
	}

	result, taskErr := relay.RelayTaskSubmit(c, relayInfo)
	if taskErr != nil {
		failLocalQueuedVideoTask(ctx, task, taskErr.Message)
		if taskErr.Error != nil {
			return taskErr.Error
		}
		return fmt.Errorf("%s", taskErr.Message)
	}
	if result == nil || strings.TrimSpace(result.UpstreamTaskID) == "" {
		err = fmt.Errorf("upstream task_id is empty")
		failLocalQueuedVideoTask(ctx, task, err.Error())
		return err
	}

	if result.Quota != task.Quota {
		service.RecalculateTaskQuota(ctx, task, result.Quota, "提交号池后调整计费")
	}

	task.PrivateData.SubmitContentSummary = buildTaskContentSummary(task, task.PrivateData.SubmitRequestBody)
	task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
	task.PrivateData.SubmitRequestBody = nil
	task.PrivateData.LastSubmitError = ""
	task.PrivateData.SubmitAttempts++
	task.Data = result.TaskData
	task.Status = model.TaskStatusSubmitted
	task.Progress = "10%"
	task.UpdatedAt = time.Now().Unix()
	if err := task.Update(); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("local queued video task %s submitted to upstream task %s", task.TaskID, result.UpstreamTaskID))
	return nil
}

func buildLocalQueuedSubmitContext(task *model.Task, channelModel *model.Channel) (*gin.Context, *relaycommon.RelayInfo, error) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req, err := http.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(task.PrivateData.SubmitRequestBody))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	common.SetContextKey(c, constant.ContextKeyUserId, task.UserId)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyUserGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyTokenId, task.PrivateData.TokenId)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	if task.Properties.OriginModelName != "" {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, task.Properties.OriginModelName)
	}
	if apiErr := middleware.SetupContextForSelectedChannel(c, channelModel, task.Properties.OriginModelName); apiErr != nil {
		return nil, nil, apiErr.Err
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	relayInfo.TaskRelayInfo.SkipPreConsume = true
	relayInfo.PublicTaskID = task.TaskID
	return c, relayInfo, nil
}

func failLocalQueuedVideoTask(ctx context.Context, task *model.Task, reason string) {
	task.PrivateData.SubmitRequestBody = nil
	task.PrivateData.LastSubmitError = reason
	task.PrivateData.SubmitAttempts++
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FailReason = reason
	task.FinishTime = time.Now().Unix()
	if err := task.Update(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("mark local queued video task %s failed: %s", task.TaskID, err.Error()))
		return
	}
	service.RefundTaskQuota(ctx, task, reason)
}
