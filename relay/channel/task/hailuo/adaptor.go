package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// https://platform.minimaxi.com/docs/api-reference/video-generation-intro
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s%s", a.baseURL, TextToVideoEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var hResp VideoResponse
	if err := common.Unmarshal(responseBody, &hResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if hResp.BaseResp.StatusCode != StatusSuccess {
		taskErr = service.TaskErrorWrapper(
			fmt.Errorf("hailuo api error: %s", hResp.BaseResp.StatusMsg),
			strconv.Itoa(hResp.BaseResp.StatusCode),
			http.StatusBadRequest,
		)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return hResp.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoRequest, error) {
	if isMiniMaxH3Model(info.UpstreamModelName) || isMiniMaxH3Model(req.Model) {
		return a.convertToMiniMaxH3RequestPayload(req, info)
	}

	modelConfig := GetModelConfig(info.UpstreamModelName)
	duration := DefaultDuration
	if req.Duration > 0 {
		duration = req.Duration
	}
	resolution := modelConfig.DefaultResolution
	if req.Size != "" {
		resolution = a.parseResolutionFromSize(req.Size, modelConfig)
	}

	videoRequest := &VideoRequest{
		Model:      info.UpstreamModelName,
		Prompt:     req.Prompt,
		Duration:   &duration,
		Resolution: resolution,
	}
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to video request failed")
	}

	return videoRequest, nil
}

func (a *TaskAdaptor) convertToMiniMaxH3RequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*VideoRequest, error) {
	duration := resolveMiniMaxH3Duration(req)
	if duration < 5 || duration > 15 {
		return nil, fmt.Errorf("duration must be between 5 and 15 seconds")
	}

	width, height, err := resolveMiniMaxH3Size(req.Size, req.AspectRatio)
	if err != nil {
		return nil, err
	}

	imageURLs := compactNonEmpty(req.Images)
	if len(imageURLs) > 5 {
		return nil, fmt.Errorf("image_urls supports at most 5 images")
	}

	hasStartEndFrame := strings.TrimSpace(req.StartImageURL) != "" || strings.TrimSpace(req.EndImageURL) != ""
	if hasStartEndFrame {
		if len(imageURLs) > 0 {
			return nil, fmt.Errorf("start/end frame mode cannot be mixed with image reference mode")
		}
		if strings.TrimSpace(req.AudioURL) != "" {
			return nil, fmt.Errorf("start/end frame mode does not support audio_url")
		}
	}
	if strings.TrimSpace(req.AudioURL) != "" && len(imageURLs) == 0 {
		return nil, fmt.Errorf("audio_url requires at least one image reference")
	}

	videoRequest := &VideoRequest{
		Model:         ModelMiniMaxH3,
		Prompt:        req.Prompt,
		Duration:      &duration,
		Width:         width,
		Height:        height,
		StartImageURL: strings.TrimSpace(req.StartImageURL),
		EndImageURL:   strings.TrimSpace(req.EndImageURL),
		AudioURL:      strings.TrimSpace(req.AudioURL),
	}
	if len(imageURLs) == 1 {
		videoRequest.ImageURL = imageURLs[0]
	} else if len(imageURLs) > 1 {
		videoRequest.ImageURLs = imageURLs
	}
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to minimax-h3 request failed")
	}

	return videoRequest, nil
}

func resolveMiniMaxH3Duration(req *relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if req.Seconds != "" {
		if seconds, err := strconv.Atoi(req.Seconds); err == nil && seconds > 0 {
			return seconds
		}
	}
	return 5
}

func resolveMiniMaxH3Size(size string, aspectRatio string) (int, int, error) {
	if strings.TrimSpace(size) != "" {
		width, height, err := parseMiniMaxH3Size(size)
		if err != nil {
			return 0, 0, err
		}
		return normalizeMiniMaxH3Size(width, height)
	}

	switch strings.TrimSpace(aspectRatio) {
	case "", "16:9":
		return 2560, 1440, nil
	case "9:16":
		return 1440, 2560, nil
	case "1:1":
		return 1440, 1440, nil
	case "4:3":
		return 1920, 1440, nil
	case "3:4":
		return 1440, 1920, nil
	case "21:9":
		return 3360, 1440, nil
	default:
		return 0, 0, fmt.Errorf("aspect_ratio is invalid")
	}
}

func parseMiniMaxH3Size(size string) (int, int, error) {
	normalized := strings.ToLower(strings.TrimSpace(size))
	normalized = strings.ReplaceAll(normalized, "×", "x")
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size must be widthxheight")
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, fmt.Errorf("size width is invalid")
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, fmt.Errorf("size height is invalid")
	}
	return width, height, nil
}

func normalizeMiniMaxH3Size(width int, height int) (int, int, error) {
	switch {
	case width == 2560 && height == 1440:
		return 2560, 1440, nil
	case width == 1280 && height == 720:
		return 2560, 1440, nil
	case width == 1920 && height == 1080:
		return 2560, 1440, nil
	case width == 1440 && height == 2560:
		return 1440, 2560, nil
	case width == 720 && height == 1280:
		return 1440, 2560, nil
	case width == 1080 && height == 1920:
		return 1440, 2560, nil
	case width == 1440 && height == 1440:
		return 1440, 1440, nil
	case width == 1024 && height == 1024:
		return 1440, 1440, nil
	case width == 1920 && height == 1440:
		return 1920, 1440, nil
	case width == 1440 && height == 1080:
		return 1920, 1440, nil
	case width == 1440 && height == 1920:
		return 1440, 1920, nil
	case width == 1080 && height == 1440:
		return 1440, 1920, nil
	case width == 3360 && height == 1440:
		return 3360, 1440, nil
	default:
		return 0, 0, fmt.Errorf("unsupported size %dx%d for minimax-h3", width, height)
	}
}

func compactNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func isMiniMaxH3Model(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), ModelMiniMaxH3)
}

func (a *TaskAdaptor) parseResolutionFromSize(size string, modelConfig ModelConfig) string {
	switch {
	case strings.Contains(size, "1080"):
		return Resolution1080P
	case strings.Contains(size, "768"):
		return Resolution768P
	case strings.Contains(size, "720"):
		return Resolution720P
	case strings.Contains(size, "512"):
		return Resolution512P
	default:
		return modelConfig.DefaultResolution
	}
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := QueryTaskResponse{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{}

	if resTask.BaseResp.StatusCode == StatusSuccess {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.BaseResp.StatusCode
		taskResult.Reason = resTask.BaseResp.StatusMsg
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}

	switch resTask.Status {
	case TaskStatusPreparing, TaskStatusQueueing, TaskStatusProcessing:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if resTask.Status == TaskStatusProcessing {
			taskResult.Progress = "50%"
		}
	case TaskStatusSuccess:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
	case TaskStatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var hailuoResp QueryTaskResponse
	if err := common.Unmarshal(originTask.Data, &hailuoResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal hailuo task data failed")
	}

	openAIVideo := originTask.ToOpenAIVideo()
	if hailuoResp.BaseResp.StatusCode != StatusSuccess {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: hailuoResp.BaseResp.StatusMsg,
			Code:    strconv.Itoa(hailuoResp.BaseResp.StatusCode),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

func (a *TaskAdaptor) buildVideoURL(_, fileID string) string {
	if a.apiKey == "" || a.baseURL == "" {
		return ""
	}

	url := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, fileID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var retrieveResp RetrieveFileResponse
	if err := common.Unmarshal(responseBody, &retrieveResp); err != nil {
		return ""
	}

	if retrieveResp.BaseResp.StatusCode != StatusSuccess {
		return ""
	}

	return retrieveResp.File.DownloadURL
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
