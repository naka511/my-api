package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	RequestID          string `json:"request_id,omitempty"`
	PollURL            string `json:"poll_url,omitempty"`
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	URL                string `json:"url,omitempty"`
	VideoURL           string `json:"video_url,omitempty"`
	ResultURL          string `json:"result_url,omitempty"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

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

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds, _ := strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	if a.useAsyncVideoAPI(upstreamModelName(info)) {
		return fmt.Sprintf("%s/v1/video/async-generations", a.baseURL), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			modelName := upstreamModelName(info)
			bodyMap["model"] = modelName
			if a.useAsyncVideoAPI(modelName) {
				normalizeLinkSkyAsyncVideoBody(bodyMap)
			}
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.ReaderOnly(storage), nil
}

func normalizeLinkSkyAsyncVideoBody(body map[string]interface{}) {
	if model, ok := body["model"].(string); ok {
		switch model {
		case "sora-2":
			body["model"] = "sora2"
		case "sora-2-pro":
			body["model"] = "sora2-pro"
		}
	}
	if duration, ok := linkSkyDurationValue(body["duration"]); ok {
		body["duration"] = duration
	} else if seconds, ok := linkSkyDurationValue(body["seconds"]); ok {
		body["duration"] = seconds
	}
	if _, ok := body["aspect_ratio"]; !ok {
		if size, ok := body["size"].(string); ok {
			switch size {
			case "1280x720", "1792x1024":
				body["aspect_ratio"] = "16:9"
			case "720x1280", "1024x1792":
				body["aspect_ratio"] = "9:16"
			}
		}
	}
	if _, ok := body["image_url"]; !ok {
		if inputReference, ok := body["input_reference"].(string); ok && strings.TrimSpace(inputReference) != "" {
			body["image_url"] = inputReference
		}
	}
	if _, ok := body["async"]; !ok {
		body["async"] = true
	}
}

func linkSkyDurationValue(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		duration, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return duration, true
	default:
		return 0, false
	}
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := dResp.upstreamTaskID()
	if upstreamID == "" {
		upstreamID = upstreamTaskIDFromResponseBody(responseBody)
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

func (r responseTask) upstreamTaskID() string {
	for _, candidate := range []string{
		r.ID,
		r.TaskID,
		r.RequestID,
		taskIDFromPollURL(r.PollURL),
	} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}

	if len(r.Data) == 0 {
		return ""
	}
	var nested responseTask
	if err := common.Unmarshal(r.Data, &nested); err != nil {
		return ""
	}
	return nested.upstreamTaskID()
}

func upstreamTaskIDFromResponseBody(responseBody []byte) string {
	return upstreamTaskIDFromJSON(gjson.ParseBytes(responseBody))
}

func upstreamTaskIDFromJSON(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	if result.IsObject() {
		for _, key := range []string{
			"id",
			"task_id",
			"taskId",
			"request_id",
			"requestId",
			"generation_id",
			"generationId",
			"job_id",
			"jobId",
			"uuid",
		} {
			if candidate := strings.TrimSpace(result.Get(key).String()); candidate != "" {
				return candidate
			}
		}
		for _, key := range []string{"poll_url", "pollUrl"} {
			if candidate := taskIDFromPollURL(result.Get(key).String()); candidate != "" {
				return candidate
			}
		}
		for _, key := range []string{"data", "result", "task", "video", "generation", "output"} {
			if candidate := upstreamTaskIDFromJSON(result.Get(key)); candidate != "" {
				return candidate
			}
		}
		var candidate string
		result.ForEach(func(_, value gjson.Result) bool {
			if !value.IsObject() && !value.IsArray() {
				return true
			}
			candidate = upstreamTaskIDFromJSON(value)
			return candidate == ""
		})
		return candidate
	}
	if result.IsArray() {
		var candidate string
		result.ForEach(func(_, value gjson.Result) bool {
			candidate = upstreamTaskIDFromJSON(value)
			return candidate == ""
		})
		return candidate
	}
	if result.Type == gjson.String {
		return strings.TrimSpace(result.String())
	}
	return ""
}

func taskIDFromPollURL(pollURL string) string {
	pollURL = strings.TrimSpace(pollURL)
	if pollURL == "" {
		return ""
	}
	if beforeQuery, _, found := strings.Cut(pollURL, "?"); found {
		pollURL = beforeQuery
	}
	pollURL = strings.TrimRight(pollURL, "/")
	if pollURL == "" {
		return ""
	}
	parts := strings.Split(pollURL, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)
	modelName, _ := body["model"].(string)
	if a.useAsyncVideoAPI(modelName) {
		uri = fmt.Sprintf("%s/v1/video/async-generations/%s", baseUrl, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) useAsyncVideoAPI(modelNames ...string) bool {
	for _, modelName := range modelNames {
		if isLinkSkyAsyncVideoModel(modelName) {
			return true
		}
	}
	return a.ChannelType == constant.ChannelTypeSora ||
		strings.Contains(strings.ToLower(a.baseURL), "linksky.top")
}

func upstreamModelName(info *relaycommon.RelayInfo) string {
	if info != nil && info.ChannelMeta != nil {
		return info.UpstreamModelName
	}
	return ""
}

func isLinkSkyAsyncVideoModel(modelName string) bool {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "video-2.0", "video-2.0-fast", "sora2", "sora2-pro", "veo31", "veo31-fast", "veo31-ref", "kling-v3", "grok-imagine-video", "ko3":
		return true
	default:
		return false
	}
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	status := taskStatusFromResponseBody(respBody)
	if status == "" {
		status = resTask.Status
	}

	if status == "" && resultURLFromResponseBody(respBody) != "" {
		status = "completed"
	}
	if status == "" && successLikeResponseBody(respBody) {
		status = "in_progress"
	}

	switch normalizeTaskStatus(status) {
	case "queued":
		taskResult.Status = model.TaskStatusQueued
	case "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = resultURLFromResponseBody(respBody)
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		if reason := errorMessageFromResponseBody(respBody); reason != "" {
			taskResult.Reason = reason
		} else if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	progress := progressFromResponseBody(respBody)
	if progress > 0 && progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", progress)
	}

	return &taskResult, nil
}

func taskStatusFromResponseBody(respBody []byte) string {
	result := gjson.ParseBytes(respBody)
	for _, path := range []string{
		"status",
		"state",
		"task_status",
		"taskStatus",
		"status_code",
		"statusCode",
		"data.status",
		"data.state",
		"data.task_status",
		"data.taskStatus",
		"data.status_code",
		"data.statusCode",
		"data.output.status",
		"data.output.task_status",
		"result.status",
		"result.state",
		"result.task_status",
		"result.taskStatus",
		"task.status",
		"task.state",
		"task.task_status",
		"task.taskStatus",
		"output.status",
		"output.task_status",
	} {
		if status := strings.TrimSpace(result.Get(path).String()); status != "" {
			return strings.ToLower(status)
		}
	}
	return ""
}

func normalizeTaskStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.ReplaceAll(status, "-", "_")
	status = strings.ReplaceAll(status, " ", "_")
	switch status {
	case "queued", "queueing", "pending", "submitted", "created", "not_start", "not_started":
		return "queued"
	case "processing", "in_progress", "running", "generating", "working", "started":
		return "in_progress"
	case "completed", "complete", "success", "succeeded", "succeed", "finished", "done":
		return "completed"
	case "failed", "failure", "fail", "error", "errored", "cancelled", "canceled":
		return "failed"
	default:
		return status
	}
}

func progressFromResponseBody(respBody []byte) int {
	result := gjson.ParseBytes(respBody)
	for _, path := range []string{"progress", "data.progress", "result.progress", "task.progress"} {
		progress := result.Get(path)
		if progress.Exists() {
			return int(progress.Int())
		}
	}
	return 0
}

func resultURLFromResponseBody(respBody []byte) string {
	result := gjson.ParseBytes(respBody)
	for _, path := range []string{
		"video_url",
		"url",
		"result_url",
		"data.video_url",
		"data.url",
		"data.result_url",
		"data.0.url",
		"data.data.0.url",
		"data.output.video_url",
		"data.output.url",
		"data.output.result_url",
		"data.output.0.url",
		"data.result.video_url",
		"data.result.url",
		"data.result.result_url",
		"data.result.0.url",
		"result.video_url",
		"result.url",
		"result.result_url",
		"result.data.0.url",
		"output.video_url",
		"output.url",
		"output.result_url",
		"output.data.0.url",
		"task.video_url",
		"task.url",
		"task.result_url",
	} {
		if url := strings.TrimSpace(result.Get(path).String()); url != "" {
			return url
		}
	}
	return ""
}

func successLikeResponseBody(respBody []byte) bool {
	result := gjson.ParseBytes(respBody)
	for _, path := range []string{"code", "message", "status_code", "statusCode"} {
		value := strings.ToLower(strings.TrimSpace(result.Get(path).String()))
		switch value {
		case "success", "ok", "0":
			return true
		}
	}
	return false
}

func errorMessageFromResponseBody(respBody []byte) string {
	result := gjson.ParseBytes(respBody)
	for _, path := range []string{
		"error.message",
		"data.error.message",
		"result.error.message",
		"task.error.message",
		"error",
		"data.error",
	} {
		if message := strings.TrimSpace(result.Get(path).String()); message != "" {
			return message
		}
	}
	return ""
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	return data, nil
}
