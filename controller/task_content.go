package controller

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	taskContentMaxStringLength = 600
	taskContentMaxRawLength    = 4000
	taskContentMaxItems        = 20
	taskContentPreviewLength   = 12
	taskContentPreviewBodyMax  = 512 * 1024
)

func GetTaskContent(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exist, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exist {
		common.ApiError(c, fmt.Errorf("task not found"))
		return
	}

	common.ApiSuccess(c, buildTaskContentSummary(task, nil))
}

func buildTaskContentSummary(task *model.Task, requestBody []byte) map[string]any {
	summary := map[string]any{
		"task_id": task.TaskID,
		"model":   firstNonEmpty(task.Properties.OriginModelName, task.Properties.UpstreamModelName),
		"action":  task.Action,
		"status":  task.Status,
		"input":   truncateTaskContentString(task.Properties.Input, taskContentMaxStringLength),
	}
	if task.GetResultURL() != "" {
		summary["result_url"] = task.GetResultURL()
	}
	if len(task.PrivateData.SubmitContentSummary) > 0 {
		for key, value := range task.PrivateData.SubmitContentSummary {
			summary[key] = value
		}
		return summary
	}
	if len(requestBody) == 0 {
		requestBody = task.PrivateData.SubmitRequestBody
	}
	if len(requestBody) == 0 {
		return summary
	}

	var payload any
	if err := common.Unmarshal(requestBody, &payload); err != nil {
		if prompt := extractPromptFromRawRequestBody(requestBody); prompt != "" {
			summary["prompt"] = truncateTaskContentString(prompt, taskContentMaxStringLength)
		}
		summary["request_body_preview"] = truncateTaskContentString(string(requestBody), taskContentMaxRawLength)
		return summary
	}

	collector := &taskContentCollector{}
	collector.walk(payload, "")
	if collector.Prompt != "" {
		summary["prompt"] = collector.Prompt
	}
	if len(collector.Images) > 0 {
		summary["images"] = collector.Images
		summary["image_count"] = len(collector.Images)
	}
	if len(collector.Videos) > 0 {
		summary["videos"] = collector.Videos
		summary["video_count"] = len(collector.Videos)
	}
	if len(collector.Params) > 0 {
		summary["params"] = collector.Params
	}
	summary["request_body_preview"] = truncateTaskContentString(common.GetJsonString(payload), taskContentMaxRawLength)
	return summary
}

func buildTaskContentPreview(task *model.Task) string {
	if task == nil {
		return ""
	}
	if preview := taskContentPromptFromSummary(task.PrivateData.SubmitContentSummary); preview != "" {
		return truncateTaskContentRunes(preview, taskContentPreviewLength)
	}
	if preview := strings.TrimSpace(task.Properties.Input); preview != "" {
		return truncateTaskContentRunes(preview, taskContentPreviewLength)
	}
	if len(task.Data) > 0 {
		var payload any
		if err := common.Unmarshal(task.Data, &payload); err == nil {
			collector := &taskContentCollector{}
			collector.walk(payload, "")
			if collector.Prompt != "" {
				return truncateTaskContentRunes(collector.Prompt, taskContentPreviewLength)
			}
		}
	}
	if len(task.PrivateData.SubmitRequestBody) == 0 || len(task.PrivateData.SubmitRequestBody) > taskContentPreviewBodyMax {
		return ""
	}
	if preview := extractPromptFromRawRequestBody(task.PrivateData.SubmitRequestBody); preview != "" {
		return truncateTaskContentRunes(preview, taskContentPreviewLength)
	}

	var payload any
	if err := common.Unmarshal(task.PrivateData.SubmitRequestBody, &payload); err != nil {
		return ""
	}
	collector := &taskContentCollector{}
	collector.walk(payload, "")
	return truncateTaskContentRunes(collector.Prompt, taskContentPreviewLength)
}

func extractPromptFromRawRequestBody(body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return ""
	}
	if values, err := url.ParseQuery(raw); err == nil {
		for _, key := range []string{"prompt", "input", "text", "content"} {
			if value := strings.TrimSpace(values.Get(key)); value != "" {
				return value
			}
		}
	}
	for _, key := range []string{"prompt", "input", "text", "content"} {
		if value := extractMultipartTextField(raw, key); value != "" {
			return value
		}
	}
	return ""
}

func extractMultipartTextField(raw string, field string) string {
	marker := fmt.Sprintf(`name="%s"`, field)
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return ""
	}
	part := raw[idx+len(marker):]
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		start := strings.Index(part, sep)
		if start < 0 {
			continue
		}
		value := part[start+len(sep):]
		for _, endSep := range []string{"\r\n--", "\n--"} {
			if end := strings.Index(value, endSep); end >= 0 {
				value = value[:end]
				break
			}
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func taskContentPromptFromSummary(summary map[string]any) string {
	if len(summary) == 0 {
		return ""
	}
	collector := &taskContentCollector{}
	collector.walk(summary, "")
	return collector.Prompt
}

type taskContentCollector struct {
	Prompt string
	Images []string
	Videos []string
	Params map[string]any
}

func (c *taskContentCollector) walk(value any, key string) {
	switch v := value.(type) {
	case map[string]any:
		for childKey, childValue := range v {
			c.walk(childValue, childKey)
		}
	case []any:
		for _, item := range v {
			c.walk(item, key)
		}
	case string:
		c.collectString(key, v)
	case float64, bool:
		c.collectParam(key, v)
	}
}

func (c *taskContentCollector) collectString(key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	lowerKey := strings.ToLower(key)
	lowerValue := strings.ToLower(value)
	if strings.Contains(lowerValue, ";base64,") || (len(value) > 1000 && !strings.Contains(value, "://")) {
		c.collectParam(key, fmt.Sprintf("[large content omitted, %d chars]", len(value)))
		return
	}
	if strings.Contains(lowerKey, "prompt") || lowerKey == "input" || lowerKey == "text" || lowerKey == "content" {
		if c.Prompt == "" {
			c.Prompt = truncateTaskContentString(value, taskContentMaxStringLength)
		}
		return
	}
	if strings.Contains(lowerKey, "image") || looksLikeImageURL(lowerValue) {
		c.Images = appendLimited(c.Images, truncateTaskContentString(value, taskContentMaxStringLength))
		return
	}
	if strings.Contains(lowerKey, "video") || looksLikeVideoURL(lowerValue) {
		c.Videos = appendLimited(c.Videos, truncateTaskContentString(value, taskContentMaxStringLength))
		return
	}
	c.collectParam(key, truncateTaskContentString(value, taskContentMaxStringLength))
}

func (c *taskContentCollector) collectParam(key string, value any) {
	if key == "" {
		return
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	if len(c.Params) >= taskContentMaxItems {
		return
	}
	c.Params[key] = value
}

func appendLimited(items []string, item string) []string {
	if len(items) >= taskContentMaxItems {
		return items
	}
	return append(items, item)
}

func looksLikeImageURL(value string) bool {
	return strings.Contains(value, "://") &&
		(strings.Contains(value, ".png") ||
			strings.Contains(value, ".jpg") ||
			strings.Contains(value, ".jpeg") ||
			strings.Contains(value, ".webp") ||
			strings.Contains(value, ".gif"))
}

func looksLikeVideoURL(value string) bool {
	return strings.Contains(value, "://") &&
		(strings.Contains(value, ".mp4") ||
			strings.Contains(value, ".webm") ||
			strings.Contains(value, ".mov") ||
			strings.Contains(value, ".m4v"))
}

func truncateTaskContentString(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + fmt.Sprintf("... [truncated, %d chars]", len(value))
}

func truncateTaskContentRunes(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen]) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
