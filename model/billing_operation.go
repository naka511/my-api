package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const billingOperationApplied = "applied"

// BillingOperation is the durable idempotency marker for a single billing
// side effect. Each effect has its own key so a partial failure can safely
// retry only the missing effect.
type BillingOperation struct {
	ID            int64  `gorm:"primaryKey"`
	OperationKey  string `gorm:"type:varchar(191);uniqueIndex"`
	TaskID        string `gorm:"type:varchar(191);index"`
	OperationType string `gorm:"type:varchar(40);index"`
	EffectType    string `gorm:"type:varchar(40);index"`
	Status        string `gorm:"type:varchar(20);index"`
	CreatedAt     int64  `gorm:"index"`
	UpdatedAt     int64  `gorm:"index"`
}

// ApplyBillingDBOperation executes one database-backed billing effect exactly
// once. The marker and the effect are committed in the same transaction, so a
// process crash cannot leave a successful effect looking unfinished.
func ApplyBillingDBOperation(operationKey, taskID, operationType, effectType string, apply func(tx *gorm.DB) error) (bool, error) {
	if operationKey == "" || apply == nil {
		return false, errors.New("billing operation key and callback are required")
	}

	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		op := &BillingOperation{
			OperationKey:  operationKey,
			TaskID:        taskID,
			OperationType: operationType,
			EffectType:    effectType,
			Status:        "pending",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(op)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where("operation_key = ?", operationKey).First(op).Error; err != nil {
				return err
			}
		}
		if op.Status == billingOperationApplied {
			return nil
		}

		// A pending marker should only survive an externally interrupted
		// transaction. Reclaim it after a short lease if one is encountered.
		if op.Status == "processing" && now-op.UpdatedAt < int64((10*time.Minute).Seconds()) {
			return nil
		}
		if err := tx.Model(&BillingOperation{}).Where("id = ?", op.ID).Updates(map[string]any{
			"status":     "processing",
			"updated_at": now,
		}).Error; err != nil {
			return err
		}

		if err := apply(tx); err != nil {
			return err
		}
		if err := tx.Model(&BillingOperation{}).Where("id = ?", op.ID).Updates(map[string]any{
			"status":     billingOperationApplied,
			"updated_at": common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

type TaskFundingEffectParams struct {
	OperationKey       string
	TaskID             string
	OperationType      string
	UserID             int
	BillingSource      string
	SubscriptionID     int
	Delta              int
	TaskDatabaseID     int64
	OldTaskQuota       int
	NewTaskQuota       int
	UpdateTaskQuota    bool
}

// ApplyUserQuotaEffectOnce is used for refunds that happen before a task row
// exists. The request-scoped key makes the wallet increment safe to retry.
func ApplyUserQuotaEffectOnce(operationKey string, userID int, delta int) (bool, error) {
	return ApplyBillingDBOperation(operationKey, "", "pre_submit_refund", "funding", func(tx *gorm.DB) error {
		if delta == 0 {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("quota", gorm.Expr("quota + ?", delta)).Error
	})
}

func ApplySubscriptionDeltaOnce(operationKey string, subscriptionID int, delta int64) (bool, error) {
	return ApplyBillingDBOperation(operationKey, "", "pre_submit_refund", "subscription", func(tx *gorm.DB) error {
		if subscriptionID <= 0 || delta == 0 {
			return nil
		}
		var sub UserSubscription
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", subscriptionID).First(&sub).Error; err != nil {
			return err
		}
		newUsed := sub.AmountUsed + delta
		if newUsed < 0 {
			newUsed = 0
		}
		if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
			return fmt.Errorf("subscription used exceeds total, used=%d total=%d", newUsed, sub.AmountTotal)
		}
		return tx.Model(&UserSubscription{}).Where("id = ?", subscriptionID).Update("amount_used", newUsed).Error
	})
}

// ApplyTaskFundingEffectOnce adjusts wallet/subscription funding. When
// UpdateTaskQuota is set, the task quota update is part of the same transaction.
func ApplyTaskFundingEffectOnce(p TaskFundingEffectParams) (bool, error) {
	return ApplyBillingDBOperation(p.OperationKey+":funding", p.TaskID, p.OperationType, "funding", func(tx *gorm.DB) error {
		if p.BillingSource == "subscription" {
			if p.SubscriptionID <= 0 {
				return errors.New("invalid subscription id")
			}
			var sub UserSubscription
			if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", p.SubscriptionID).First(&sub).Error; err != nil {
				return err
			}
			newUsed := sub.AmountUsed + int64(p.Delta)
			if newUsed < 0 {
				newUsed = 0
			}
			if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
				return fmt.Errorf("subscription used exceeds total, used=%d total=%d", newUsed, sub.AmountTotal)
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", p.SubscriptionID).Update("amount_used", newUsed).Error; err != nil {
				return err
			}
		} else if p.Delta > 0 {
			if err := tx.Model(&User{}).Where("id = ?", p.UserID).Update("quota", gorm.Expr("quota - ?", p.Delta)).Error; err != nil {
				return err
			}
		} else if p.Delta < 0 {
			if err := tx.Model(&User{}).Where("id = ?", p.UserID).Update("quota", gorm.Expr("quota + ?", -p.Delta)).Error; err != nil {
				return err
			}
		}

		if p.UpdateTaskQuota && p.TaskDatabaseID != 0 && p.OldTaskQuota != p.NewTaskQuota {
			result := tx.Model(&Task{}).Where("id = ? AND quota = ?", p.TaskDatabaseID, p.OldTaskQuota).Update("quota", p.NewTaskQuota)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("task quota changed before billing settlement")
			}
		}
		return nil
	})
}

type TokenQuotaEffectParams struct {
	OperationKey string
	TaskID       string
	OperationType string
	TokenID      int
	TokenKey     string
	Delta        int
}

func ApplyTokenQuotaEffectOnce(p TokenQuotaEffectParams) (bool, error) {
	applied, err := ApplyBillingDBOperation(p.OperationKey+":token", p.TaskID, p.OperationType, "token", func(tx *gorm.DB) error {
		if p.TokenID <= 0 || p.Delta == 0 {
			return nil
		}
		return tx.Model(&Token{}).Where("id = ?", p.TokenID).Updates(map[string]any{
			"remain_quota": gorm.Expr("remain_quota - ?", p.Delta),
			"used_quota":   gorm.Expr("used_quota + ?", p.Delta),
			"accessed_time": common.GetTimestamp(),
		}).Error
	})
	if err == nil && applied && common.RedisEnabled && p.TokenKey != "" && p.Delta != 0 {
		increment := int64(-p.Delta)
		gopool.Go(func() {
			if cacheErr := cacheIncrTokenQuota(p.TokenKey, increment); cacheErr != nil {
				common.SysLog("failed to update token quota cache after billing operation: " + cacheErr.Error())
			}
		})
	}
	return applied, err
}

type UsageStatsEffectParams struct {
	OperationKey string
	TaskID       string
	OperationType string
	UserID       int
	ChannelID    int
	Delta        int
}

func ApplyUsageStatsEffectOnce(p UsageStatsEffectParams) (bool, error) {
	return ApplyBillingDBOperation(p.OperationKey+":stats", p.TaskID, p.OperationType, "usage_stats", func(tx *gorm.DB) error {
		if p.UserID != 0 && p.Delta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", p.UserID).Update("used_quota", gorm.Expr("used_quota + ?", p.Delta)).Error; err != nil {
				return err
			}
		}
		if p.ChannelID != 0 && p.Delta != 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", p.ChannelID).Update("used_quota", gorm.Expr("used_quota + ?", p.Delta)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type RequestUsageEffectParams struct {
	OperationKey string
	TaskID       string
	UserID       int
	ChannelID    int
	Quota        int
}

func ApplyRequestUsageEffectOnce(p RequestUsageEffectParams) (bool, error) {
	return ApplyBillingDBOperation(p.OperationKey+":stats", p.TaskID, "submit", "usage_stats", func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", p.UserID).Updates(map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", p.Quota),
			"request_count": gorm.Expr("request_count + ?", 1),
		}).Error; err != nil {
			return err
		}
		if p.ChannelID != 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", p.ChannelID).Update("used_quota", gorm.Expr("used_quota + ?", p.Quota)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
