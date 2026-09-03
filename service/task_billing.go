package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	billingMode := billing_setting.GetBillingMode(info.OriginModelName)
	// fixed_price 是明确的固定收费模式，不参与 seconds / duration 等参数倍率。
	if billingMode == billing_setting.BillingModePerSecond {
		logContent = fmt.Sprintf("%s，按秒收费", logContent)
	} else if billingMode == billing_setting.BillingModeFixedPrice {
		logContent = fmt.Sprintf("%s，固定收费", logContent)
	} else if info.PriceData.UsePrice {
		// 兼容旧 ModelPrice 配置：未显式保存 billing_mode 时仍按固定收费处理。
		logContent = fmt.Sprintf("%s，固定收费", logContent)
	} else if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if len(info.PriceData.OtherRatios) > 0 {
			var contents []string
			for key, ra := range info.PriceData.OtherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	if info.TaskRelayInfo != nil && info.PublicTaskID != "" {
		other["task_id"] = info.PublicTaskID
	}
	other["request_path"] = c.Request.URL.Path
	other["billing_mode"] = billingMode
	other["model_price"] = info.PriceData.ModelPrice
	for key, value := range info.PriceData.OtherRatios {
		other[key] = value
	}
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
		BillingOperationKey: func() string {
			if info.TaskRelayInfo != nil && info.PublicTaskID != "" {
				return taskBillingOperationKeyFromID(info.PublicTaskID, "submit") + ":log"
			}
			return ""
		}(),
	})
	if info.TaskRelayInfo != nil && info.PublicTaskID != "" {
		if _, err := model.ApplyRequestUsageEffectOnce(model.RequestUsageEffectParams{
			OperationKey: taskBillingOperationKeyFromID(info.PublicTaskID, "submit"),
			TaskID:       info.PublicTaskID,
			UserID:       info.UserId,
			ChannelID:    info.ChannelId,
			Quota:        info.PriceData.Quota,
		}); err != nil {
			logger.LogError(c, fmt.Sprintf("更新任务提交累计消耗失败 task=%s: %s", info.PublicTaskID, err.Error()))
		}
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
		model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
	}
}

func taskBillingOperationKeyFromID(taskID string, operationType string) string {
	if taskID == "" || operationType == "" {
		return ""
	}
	return fmt.Sprintf("task:%s:%s", taskID, operationType)
}

func EnsureTaskConsumeLog(ctx context.Context, task *model.Task) {
	if task == nil || task.Quota <= 0 || model.HasTaskConsumeLog(task.TaskID) {
		return
	}
	other := taskBillingOther(task)
	other["is_task"] = true
	other["task_id"] = task.TaskID
	other["request_path"] = "/v1/video/generations"

	content := fmt.Sprintf("异步任务完成补记消费，任务ID %s", task.TaskID)
	if task.Action != "" {
		content = fmt.Sprintf("操作 %s，异步任务完成补记消费", task.Action)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeConsume,
		Content:   content,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     task.Quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		BillingOperationKey: taskBillingOperationKey(task, "success_settle") + ":log",
	})
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskBillingOperationKey 返回任务级计费操作的稳定幂等键。
func taskBillingOperationKey(task *model.Task, operationType string) string {
	if task == nil || task.TaskID == "" {
		return ""
	}
	return fmt.Sprintf("task:%s:%s", task.TaskID, operationType)
}

func taskAdjustBillingEffects(ctx context.Context, task *model.Task, operationType string, delta int, updateTaskQuota bool, oldQuota int, newQuota int) error {
	if task == nil || delta == 0 {
		return nil
	}
	operationKey := taskBillingOperationKey(task, operationType)
	if operationKey == "" {
		return fmt.Errorf("task billing operation key is empty")
	}
	billingSource := task.PrivateData.BillingSource
	if billingSource == "" {
		billingSource = BillingSourceWallet
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		_, err = model.ApplyTaskFundingEffectOnce(model.TaskFundingEffectParams{
			OperationKey:    operationKey,
			TaskID:          task.TaskID,
			OperationType:   operationType,
			UserID:          task.UserId,
			BillingSource:   billingSource,
			SubscriptionID:  task.PrivateData.SubscriptionId,
			Delta:           delta,
			TaskDatabaseID:  task.ID,
			OldTaskQuota:    oldQuota,
			NewTaskQuota:    newQuota,
			UpdateTaskQuota: updateTaskQuota,
		})
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
		}
	}
	if err != nil {
		return err
	}

	tokenKey := ""
	if task.PrivateData.TokenId > 0 {
		tokenKey = resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	}
	var tokenErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, tokenErr = model.ApplyTokenQuotaEffectOnce(model.TokenQuotaEffectParams{
			OperationKey:  operationKey,
			TaskID:        task.TaskID,
			OperationType: operationType,
			TokenID:       task.PrivateData.TokenId,
			TokenKey:      tokenKey,
			Delta:         delta,
		})
		if tokenErr == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
		}
	}
	if tokenErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, tokenErr.Error()))
	}

	var statsErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, statsErr = model.ApplyUsageStatsEffectOnce(model.UsageStatsEffectParams{
			OperationKey:  operationKey,
			TaskID:        task.TaskID,
			OperationType: operationType,
			UserID:        task.UserId,
			ChannelID:     task.ChannelId,
			Delta:         delta,
		})
		if statsErr == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
		}
	}
	if statsErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("更新任务累计消耗失败 (delta=%d, task=%s): %s", delta, task.TaskID, statsErr.Error()))
	}
	if tokenErr != nil {
		return fmt.Errorf("token quota adjustment failed: %w", tokenErr)
	}
	if statsErr != nil {
		return fmt.Errorf("usage stats adjustment failed: %w", statsErr)
	}
	return nil
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		if bc.BillingMode != "" {
			other["billing_mode"] = bc.BillingMode
		}
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if len(bc.OtherRatios) > 0 {
			for k, v := range bc.OtherRatios {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，将预扣的 quota 退还给用户（支持钱包和订阅），并退还令牌额度。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) {
	quota := task.Quota
	if quota == 0 {
		return
	}

	operationType := "failure_refund"
	// 1. 退还资金来源（钱包或订阅），并用唯一操作键保证不会重复退款。
	if err := taskAdjustBillingEffects(ctx, task, operationType, -quota, false, 0, 0); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 2. 记录日志；日志键与退款操作绑定，重复轮询不会产生重复记录。
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		BillingOperationKey: taskBillingOperationKey(task, operationType) + ":log",
	})
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string) {
	if actualQuota <= 0 {
		return
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按次/固定计费，跳过差额结算（保持预扣：%s，候选实际：%s，%s）",
			task.TaskID,
			logger.LogQuota(task.Quota),
			logger.LogQuota(actualQuota),
			reason,
		))
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	operationType := "success_settle"
	// 资金来源和任务实际额度在同一幂等事务中调整，避免金额已变但 task.quota 未更新。
	if err := taskAdjustBillingEffects(ctx, task, operationType, quotaDelta, true, preConsumedQuota, actualQuota); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	task.Quota = actualQuota

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		BillingOperationKey: taskBillingOperationKey(task, operationType) + ":log",
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.PerCallBilling {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 按次/固定计费，跳过 token 重算（tokens=%d）", task.TaskID, totalTokens))
		return
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
	// 只有配置了倍率(非固定价格)时才按 token 重新计费
	if !hasRatioSetting || modelRatio <= 0 {
		return
	}

	// 获取用户和组的倍率信息
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return
	}

	groupRatio := ratio_setting.GetGroupRatio(group)
	userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

	var finalGroupRatio float64
	if hasUserGroupRatio {
		finalGroupRatio = userGroupRatio
	} else {
		finalGroupRatio = groupRatio
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if bc := task.PrivateData.BillingContext; bc != nil {
		for _, r := range bc.OtherRatios {
			if r != 1.0 && r > 0 {
				otherMultiplier *= r
			}
		}
	}

	// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio * otherMultiplier
	actualQuota := int(float64(totalTokens) * modelRatio * finalGroupRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, finalGroupRatio, otherMultiplier)
	RecalculateTaskQuota(ctx, task, actualQuota, reason)
}
