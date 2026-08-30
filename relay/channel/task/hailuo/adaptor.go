package hailuo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	if taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	modelName := resolveMiniMaxH3ModelName(req.Model, info.UpstreamModelName)
	if modelName == "" {
		if isLegacyMiniMaxH3Model(req.Model, info.UpstreamModelName) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("minimax-h3 is no longer available; use minimax-h3-480p, minimax-h3-768p, minimax-h3-2k, or minimax-h3-4k"),
				"unsupported_model",
				http.StatusBadRequest,
			)
		}
		return nil
	}

	if err := validateMiniMaxH3Request(&req, modelName); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	return nil
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
	if strings.TrimSpace(hResp.TaskID) == "" {
		taskErr = service.TaskErrorWrapperLocal(
			fmt.Errorf("minimax-h3 submit response is missing task_id"),
			"invalid_response",
			http.StatusBadGateway,
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

	uri := fmt.Sprintf("%s%s?task_id=%s", baseUrl, QueryTaskEndpoint, url.QueryEscape(taskID))

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
	if resolveMiniMaxH3ModelName(req.Model, info.UpstreamModelName) != "" {
		return a.convertToMiniMaxH3RequestPayload(req, info)
	}
	if isLegacyMiniMaxH3Model(req.Model, info.UpstreamModelName) {
		return nil, fmt.Errorf("minimax-h3 is no longer available; use minimax-h3-480p, minimax-h3-768p, minimax-h3-2k, or minimax-h3-4k")
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
	modelName := resolveMiniMaxH3ModelName(req.Model, info.UpstreamModelName)
	if modelName == "" {
		return nil, fmt.Errorf("unsupported minimax-h3 model")
	}
	if err := validateMiniMaxH3Request(req, modelName); err != nil {
		return nil, err
	}

	duration := resolveMiniMaxH3Duration(req)

	width, height, err := resolveMiniMaxH3Size(modelName, req.Size, req.AspectRatio, req.Width, req.Height)
	if err != nil {
		return nil, err
	}

	imageURLs := miniMaxH3ImageURLs(req)

	videoRequest := &VideoRequest{
		Model:         UpstreamModelMiniMaxH3,
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
	videoRequest.ImageGuidance = toMiniMaxH3ImageGuidance(req.ImageGuidance)
	videoRequest.StartFrame = toMiniMaxH3MediaReferences(req.StartFrame)
	videoRequest.EndFrame = toMiniMaxH3MediaReferences(req.EndFrame)
	videoRequest.VideoURL = strings.TrimSpace(req.VideoURL)
	videoRequest.VideoReference = toMiniMaxH3MediaReferences(req.VideoReference)
	videoRequest.AudioReference = toMiniMaxH3MediaReferences(req.AudioReference)
	if err := req.UnmarshalMetadata(&videoRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata to minimax-h3 request failed")
	}
	videoRequest.Model = UpstreamModelMiniMaxH3
	videoRequest.Duration = &duration
	videoRequest.Width = width
	videoRequest.Height = height
	videoRequest.StartImageURL = strings.TrimSpace(req.StartImageURL)
	videoRequest.EndImageURL = strings.TrimSpace(req.EndImageURL)
	videoRequest.AudioURL = strings.TrimSpace(req.AudioURL)
	videoRequest.ImageGuidance = toMiniMaxH3ImageGuidance(req.ImageGuidance)
	videoRequest.StartFrame = toMiniMaxH3MediaReferences(req.StartFrame)
	videoRequest.EndFrame = toMiniMaxH3MediaReferences(req.EndFrame)
	videoRequest.VideoURL = strings.TrimSpace(req.VideoURL)
	videoRequest.VideoReference = toMiniMaxH3MediaReferences(req.VideoReference)
	videoRequest.AudioReference = toMiniMaxH3MediaReferences(req.AudioReference)
	videoRequest.ImageURL = ""
	videoRequest.ImageURLs = nil
	if len(imageURLs) == 1 {
		videoRequest.ImageURL = imageURLs[0]
	} else if len(imageURLs) > 1 {
		videoRequest.ImageURLs = imageURLs
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

func validateMiniMaxH3Request(req *relaycommon.TaskSubmitReq, modelName string) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if utf8.RuneCountInString(req.Prompt) > 5000 {
		return fmt.Errorf("prompt must not exceed 5000 characters")
	}

	duration := resolveMiniMaxH3Duration(req)
	if duration < 5 || duration > 15 {
		return fmt.Errorf("duration must be between 5 and 15 seconds")
	}

	if _, _, err := resolveMiniMaxH3Size(modelName, req.Size, req.AspectRatio, req.Width, req.Height); err != nil {
		return err
	}

	imageURLs := miniMaxH3ImageURLs(req)
	if len(imageURLs)+len(req.ImageGuidance) > 9 {
		return fmt.Errorf("image references support at most 9 images")
	}
	for _, guidance := range req.ImageGuidance {
		if strings.TrimSpace(guidance.URL) == "" {
			return fmt.Errorf("image_guidance url must not be empty")
		}
	}

	startImage := strings.TrimSpace(req.StartImageURL)
	endImage := strings.TrimSpace(req.EndImageURL)
	startFramePresent := len(req.StartFrame) > 0
	endFramePresent := len(req.EndFrame) > 0
	if (startImage != "") != (endImage != "") {
		return fmt.Errorf("start_image_url and end_image_url must be provided together")
	}
	if startFramePresent != endFramePresent {
		return fmt.Errorf("start_frame and end_frame must be provided together")
	}
	for _, frame := range append(append([]relaycommon.TaskMediaReference{}, req.StartFrame...), req.EndFrame...) {
		if strings.TrimSpace(frame.URL) == "" {
			return fmt.Errorf("frame url must not be empty")
		}
	}
	frameMode := startImage != "" || startFramePresent
	if frameMode {
		if len(imageURLs) > 0 || len(req.ImageGuidance) > 0 || strings.TrimSpace(req.VideoURL) != "" || len(req.VideoReference) > 0 || strings.TrimSpace(req.AudioURL) != "" || len(req.AudioReference) > 0 {
			return fmt.Errorf("multimodal references and frame mode cannot be combined")
		}
	}
	if err := validateMiniMaxH3MediaReferences(req.VideoURL, req.VideoReference, 3, 15, "video"); err != nil {
		return err
	}
	if err := validateMiniMaxH3MediaReferences(req.AudioURL, req.AudioReference, 3, 15, "audio"); err != nil {
		return err
	}
	return nil
}

func validateMiniMaxH3MediaReferences(singleURL string, references []relaycommon.TaskMediaReference, maxCount int, maxTotalDuration int, kind string) error {
	count := len(references)
	if strings.TrimSpace(singleURL) != "" {
		count++
	}
	if count > maxCount {
		return fmt.Errorf("%s references support at most %d files", kind, maxCount)
	}
	totalDuration := 0
	for _, reference := range references {
		if strings.TrimSpace(reference.URL) == "" {
			return fmt.Errorf("%s_reference url must not be empty", kind)
		}
		if reference.Duration != 0 && (reference.Duration < 1 || reference.Duration > maxTotalDuration) {
			return fmt.Errorf("%s reference duration must be between 1 and %d seconds", kind, maxTotalDuration)
		}
		totalDuration += reference.Duration
	}
	if totalDuration > maxTotalDuration {
		return fmt.Errorf("total %s reference duration must not exceed %d seconds", kind, maxTotalDuration)
	}
	return nil
}

func toMiniMaxH3ImageGuidance(guidance []relaycommon.TaskImageGuidance) []ImageGuidance {
	if len(guidance) == 0 {
		return nil
	}
	result := make([]ImageGuidance, 0, len(guidance))
	for _, item := range guidance {
		result = append(result, ImageGuidance{
			URL:      strings.TrimSpace(item.URL),
			Strength: item.Strength,
		})
	}
	return result
}

func toMiniMaxH3MediaReferences(references []relaycommon.TaskMediaReference) []MediaReference {
	if len(references) == 0 {
		return nil
	}
	result := make([]MediaReference, 0, len(references))
	for _, reference := range references {
		result = append(result, MediaReference{
			URL:      strings.TrimSpace(reference.URL),
			Duration: reference.Duration,
		})
	}
	return result
}

func miniMaxH3ImageURLs(req *relaycommon.TaskSubmitReq) []string {
	if req == nil {
		return nil
	}
	values := make([]string, 0, len(req.Images)+len(req.ImageURLs)+2)
	values = append(values, req.Image, req.ImageURL)
	values = append(values, req.Images...)
	values = append(values, req.ImageURLs...)

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func resolveMiniMaxH3Size(modelName, size string, aspectRatio string, width, height int) (int, int, error) {
	sizes, ok := miniMaxH3Sizes[modelName]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported minimax-h3 model %q", modelName)
	}
	if width != 0 || height != 0 {
		if width <= 0 || height <= 0 {
			return 0, 0, fmt.Errorf("width and height must be provided together")
		}
		if !miniMaxH3SizeAllowed(sizes, width, height) {
			return 0, 0, fmt.Errorf("unsupported size %dx%d for %s", width, height, modelName)
		}
		return width, height, nil
	}
	if strings.TrimSpace(size) != "" {
		width, height, err := parseMiniMaxH3Size(size)
		if err != nil {
			return 0, 0, err
		}
		if !miniMaxH3SizeAllowed(sizes, width, height) {
			return 0, 0, fmt.Errorf("unsupported size %dx%d for %s", width, height, modelName)
		}
		return width, height, nil
	}

	if resolved, exists := sizes[strings.TrimSpace(aspectRatio)]; exists {
		return resolved[0], resolved[1], nil
	}
	if strings.TrimSpace(aspectRatio) == "" {
		resolved := sizes["16:9"]
		return resolved[0], resolved[1], nil
	}
	return 0, 0, fmt.Errorf("aspect_ratio is invalid")
}

var miniMaxH3Sizes = map[string]map[string][2]int{
	ModelMiniMaxH3480P: {"16:9": {856, 480}, "9:16": {480, 856}, "1:1": {480, 480}, "4:3": {640, 480}, "3:4": {480, 640}, "21:9": {1120, 480}},
	ModelMiniMaxH3768P: {"16:9": {1376, 768}, "9:16": {768, 1376}, "1:1": {768, 768}, "4:3": {1024, 768}, "3:4": {768, 1024}, "21:9": {1792, 768}},
	ModelMiniMaxH32K:   {"16:9": {2560, 1440}, "9:16": {1440, 2560}, "1:1": {1440, 1440}, "4:3": {1920, 1440}, "3:4": {1440, 1920}, "21:9": {3360, 1440}},
	ModelMiniMaxH34K:   {"16:9": {3840, 2160}, "9:16": {2160, 3840}, "1:1": {2160, 2160}, "4:3": {2880, 2160}, "3:4": {2160, 2880}, "21:9": {5040, 2160}},
}

func miniMaxH3SizeAllowed(sizes map[string][2]int, width, height int) bool {
	for _, size := range sizes {
		if size[0] == width && size[1] == height {
			return true
		}
	}
	return false
}

func resolveMiniMaxH3ModelName(modelNames ...string) string {
	for _, modelName := range modelNames {
		if common.IsMiniMaxH3Model(modelName) {
			return strings.ToLower(strings.TrimSpace(modelName))
		}
	}
	return ""
}

func isLegacyMiniMaxH3Model(modelNames ...string) bool {
	for _, modelName := range modelNames {
		if strings.EqualFold(strings.TrimSpace(modelName), UpstreamModelMiniMaxH3) {
			return true
		}
	}
	return false
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

	switch strings.ToLower(strings.TrimSpace(resTask.Status)) {
	case strings.ToLower(TaskStatusPreparing), strings.ToLower(TaskStatusQueueing), strings.ToLower(TaskStatusProcessing):
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
		if strings.EqualFold(resTask.Status, TaskStatusProcessing) {
			taskResult.Progress = "50%"
		}
	case strings.ToLower(TaskStatusSuccess):
		if strings.TrimSpace(resTask.TaskID) == "" {
			return nil, fmt.Errorf("minimax-h3 success response is missing task_id")
		}
		if strings.TrimSpace(resTask.FileID) == "" {
			return nil, fmt.Errorf("minimax-h3 success response is missing file_id")
		}
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = a.buildVideoURL(resTask.TaskID, resTask.FileID)
	case strings.ToLower(TaskStatusFailed):
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
	if a.apiKey == "" || a.baseURL == "" || strings.TrimSpace(fileID) == "" {
		return ""
	}

	fileURL := fmt.Sprintf("%s/v1/files/retrieve?file_id=%s", a.baseURL, url.QueryEscape(fileID))

	req, err := http.NewRequest(http.MethodGet, fileURL, nil)
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
