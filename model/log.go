package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		pattern, err := sanitizeLikePattern(value)
		if err != nil {
			return nil, err
		}
		return tx.Where(column+" LIKE ? ESCAPE '!'", pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

type Log struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// delete(otherMap, "reject_reason")
			delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func resolveLogUsername(c *gin.Context, userId int) string {
	username := c.GetString("username")
	if username != "" {
		return username
	}
	username = common.GetContextKeyString(c, constant.ContextKeyUserName)
	if username != "" {
		return username
	}
	username, _ = GetUsernameById(userId, false)
	return username
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := resolveLogUsername(c, userId)
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
	if params.LogType == LogTypeConsume && common.DataExportEnabled {
		LogQuotaData(params.UserId, username, params.ModelName, params.Quota, common.GetTimestamp(), 0)
	}
}

func HasTaskConsumeLog(taskID string) bool {
	if taskID == "" {
		return false
	}
	var count int64
	pattern := fmt.Sprintf("%%\"task_id\":\"%s\"%%", taskID)
	if err := LOG_DB.Model(&Log{}).
		Where("type = ? AND other LIKE ?", LogTypeConsume, pattern).
		Count(&count).Error; err != nil {
		common.SysLog("failed to check task consume log: " + err.Error())
		return false
	}
	return count > 0
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota       int         `json:"quota"`
	Rpm         int         `json:"rpm"`
	Tpm         int         `json:"tpm"`
	TaskSummary TaskSummary `json:"task_summary" gorm:"-"`
}

type TaskStatusSummary struct {
	Running int64 `json:"running"`
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
}

type TaskSummary struct {
	Total TaskStatusSummary `json:"total"`
	Image TaskStatusSummary `json:"image"`
	Video TaskStatusSummary `json:"video"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	if logType != LogTypeUnknown {
		quotaQuery, queryErr := applyLogStatFilters(LOG_DB.Table("logs").Select("coalesce(sum(quota), 0)"), startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
		if queryErr != nil {
			return stat, queryErr
		}
		var quota int
		if queryErr = quotaQuery.Where("type = ?", logType).Scan(&quota).Error; queryErr != nil {
			common.SysError("failed to query selected log type quota stat: " + queryErr.Error())
			return stat, errors.New("查询统计数据失败")
		}
		stat.Quota = quota
	} else {
		consumeQuotaQuery, err := applyLogStatFilters(LOG_DB.Table("logs").Select("coalesce(sum(quota), 0)"), startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
		if err != nil {
			return stat, err
		}
		refundQuotaQuery, err := applyLogStatFilters(LOG_DB.Table("logs").Select("coalesce(sum(quota), 0)"), startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
		if err != nil {
			return stat, err
		}
		consumeQuotaQuery = consumeQuotaQuery.Where("type = ?", LogTypeConsume)
		refundQuotaQuery = refundQuotaQuery.Where("type = ?", LogTypeRefund)

		var consumeQuota int
		var refundQuota int
		if err := consumeQuotaQuery.Scan(&consumeQuota).Error; err != nil {
			common.SysError("failed to query consume quota stat: " + err.Error())
			return stat, errors.New("查询统计数据失败")
		}
		if err := refundQuotaQuery.Scan(&refundQuota).Error; err != nil {
			common.SysError("failed to query refund quota stat: " + err.Error())
			return stat, errors.New("查询统计数据失败")
		}
		stat.Quota = consumeQuota - refundQuota
	}

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0) tpm")

	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "username", username); err != nil {
		return stat, err
	}
	if tokenName != "" {
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	stat.TaskSummary, err = GetTaskSummary(startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		common.SysError("failed to query task summary stat: " + err.Error())
		stat.TaskSummary = TaskSummary{}
	}

	return stat, nil
}

func applyLogStatFilters(tx *gorm.DB, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (*gorm.DB, error) {
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return nil, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return nil, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}
	return tx, nil
}

func GetTaskSummary(startTimestamp int64, endTimestamp int64, modelName string, username string, channel int, group string) (summary TaskSummary, err error) {
	image, err := getImageTaskSummary(startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	video, err := getVideoTaskSummary(startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	summary.Image = image
	summary.Video = video
	summary.Total = TaskStatusSummary{
		Running: image.Running + video.Running,
		Success: image.Success + video.Success,
		Failed:  image.Failed + video.Failed,
	}
	return summary, nil
}

func applyTaskSummaryLogFilters(tx *gorm.DB, startTimestamp int64, endTimestamp int64, modelName string, username string, channel int, group string) (*gorm.DB, error) {
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return nil, err
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(commonGroupCol+" = ?", group)
	}
	return tx, nil
}

func getImageTaskSummary(startTimestamp int64, endTimestamp int64, modelName string, username string, channel int, group string) (TaskStatusSummary, error) {
	var summary TaskStatusSummary
	base := LOG_DB.Model(&Log{}).Where("other LIKE ?", `%"/v1/images/generations"%`)

	successTx, err := applyTaskSummaryLogFilters(base.Session(&gorm.Session{}), startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	if err := successTx.Where("type = ?", LogTypeConsume).Count(&summary.Success).Error; err != nil {
		common.SysError("failed to count image task success logs: " + err.Error())
		return summary, errors.New("查询任务汇总失败")
	}

	failedTx, err := applyTaskSummaryLogFilters(base.Session(&gorm.Session{}), startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	if err := failedTx.Where("type = ?", LogTypeError).Count(&summary.Failed).Error; err != nil {
		common.SysError("failed to count image task failed logs: " + err.Error())
		return summary, errors.New("查询任务汇总失败")
	}
	return summary, nil
}

func applyTaskSummaryVideoFilters(tx *gorm.DB, startTimestamp int64, endTimestamp int64, modelName string, username string, channel int, group string) (*gorm.DB, error) {
	if startTimestamp != 0 {
		tx = tx.Where("submit_time >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("submit_time <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}
	if modelName != "" {
		if strings.Contains(modelName, "%") {
			pattern, err := sanitizeLikePattern(modelName)
			if err != nil {
				return nil, err
			}
			tx = tx.Where("properties LIKE ? ESCAPE '!'", pattern)
		} else {
			tx = tx.Where("properties LIKE ?", "%"+modelName+"%")
		}
	}
	if username != "" {
		userIds, err := GetUserIdsByUsernameFilter(username)
		if err != nil {
			return nil, err
		}
		if len(userIds) == 0 {
			tx = tx.Where("1 = 0")
		} else {
			tx = tx.Where("user_id IN ?", userIds)
		}
	}
	return tx, nil
}

func GetUserIdsByUsernameFilter(username string) ([]int, error) {
	var users []User
	tx := DB.Model(&User{})
	if strings.Contains(username, "%") {
		pattern, err := sanitizeLikePattern(username)
		if err != nil {
			return nil, err
		}
		tx = tx.Where("username LIKE ? ESCAPE '!'", pattern)
	} else {
		tx = tx.Where("username = ?", username)
	}
	if err := tx.Select("id").Find(&users).Error; err != nil {
		common.SysError("failed to query users for task summary: " + err.Error())
		return nil, errors.New("查询任务汇总失败")
	}
	userIds := make([]int, 0, len(users))
	for _, user := range users {
		userIds = append(userIds, user.Id)
	}
	return userIds, nil
}

func getVideoTaskSummary(startTimestamp int64, endTimestamp int64, modelName string, username string, channel int, group string) (TaskStatusSummary, error) {
	var summary TaskStatusSummary
	base := DB.Model(&Task{}).Where("platform NOT IN ?", []constant.TaskPlatform{constant.TaskPlatformSuno, constant.TaskPlatformMidjourney})

	runningTx, err := applyTaskSummaryVideoFilters(base.Session(&gorm.Session{}), startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	if err := runningTx.Where("status NOT IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Count(&summary.Running).Error; err != nil {
		common.SysError("failed to count running video tasks: " + err.Error())
		return summary, errors.New("查询任务汇总失败")
	}

	successTx, err := applyTaskSummaryVideoFilters(base.Session(&gorm.Session{}), startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	if err := successTx.Where("status = ?", TaskStatusSuccess).Count(&summary.Success).Error; err != nil {
		common.SysError("failed to count success video tasks: " + err.Error())
		return summary, errors.New("查询任务汇总失败")
	}

	failedTx, err := applyTaskSummaryVideoFilters(base.Session(&gorm.Session{}), startTimestamp, endTimestamp, modelName, username, channel, group)
	if err != nil {
		return summary, err
	}
	if err := failedTx.Where("status = ?", TaskStatusFailure).Count(&summary.Failed).Error; err != nil {
		common.SysError("failed to count failed video tasks: " + err.Error())
		return summary, errors.New("查询任务汇总失败")
	}
	return summary, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
