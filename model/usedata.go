package model

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(userId int, username string, modelName string, quota int, createdAt int64, tokenUsed int) {
	key := fmt.Sprintf("%d-%s-%s-%d", userId, username, modelName, createdAt)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += quota
		quotaData.TokenUsed += tokenUsed
	} else {
		quotaData = &QuotaData{
			UserID:    userId,
			Username:  username,
			ModelName: modelName,
			CreatedAt: createdAt,
			Count:     1,
			Quota:     quota,
			TokenUsed: tokenUsed,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(userId int, username string, modelName string, quota int, createdAt int64, tokenUsed int) {
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, username, modelName, quota, createdAt, tokenUsed)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.Count, quotaData.Quota, quotaData.CreatedAt, quotaData.TokenUsed)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(userId int, username string, modelName string, count int, quota int, createdAt int64, tokenUsed int) {
	err := DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ?",
		userId, username, modelName, createdAt).Updates(map[string]interface{}{
		"count":      gorm.Expr("count + ?", count),
		"quota":      gorm.Expr("quota + ?", quota),
		"token_used": gorm.Expr("token_used + ?", tokenUsed),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetActualQuotaData(0, username, startTime, endTime)
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetActualQuotaData(userId, "", startTime, endTime)
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	data, err := GetActualQuotaData(0, "", startTime, endTime)
	if err != nil {
		return nil, err
	}

	type userTimeKey struct {
		UserID    int
		Username  string
		CreatedAt int64
	}
	grouped := make(map[userTimeKey]*QuotaData)
	for _, item := range data {
		key := userTimeKey{UserID: item.UserID, Username: item.Username, CreatedAt: item.CreatedAt}
		if existing, ok := grouped[key]; ok {
			existing.Count += item.Count
			existing.Quota += item.Quota
			existing.TokenUsed += item.TokenUsed
			continue
		}
		grouped[key] = &QuotaData{
			UserID:    item.UserID,
			Username:  item.Username,
			CreatedAt: item.CreatedAt,
			Count:     item.Count,
			Quota:     item.Quota,
			TokenUsed: item.TokenUsed,
		}
	}

	quotaDatas := make([]*QuotaData, 0, len(grouped))
	for _, item := range grouped {
		quotaDatas = append(quotaDatas, item)
	}
	sort.Slice(quotaDatas, func(i, j int) bool {
		if quotaDatas[i].CreatedAt == quotaDatas[j].CreatedAt {
			return quotaDatas[i].Username < quotaDatas[j].Username
		}
		return quotaDatas[i].CreatedAt < quotaDatas[j].CreatedAt
	})
	return quotaDatas, nil
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	return GetActualQuotaData(0, username, startTime, endTime)
}

// dashboardAggregateRow contains database-side hourly aggregates. Keeping the
// aggregation in SQL avoids loading a high-volume logs table into memory.
type dashboardAggregateRow struct {
	UserID    int
	Username  string
	ModelName string
	CreatedAt int64
	Quota     int
	TokenUsed int
	Count     int
}

const (
	dashboardTaskFlagPattern = `%"is_task":true%`
	dashboardTaskIDPattern   = `%"task_id":%`
)

func dashboardTaskModelName(task *Task) string {
	if task == nil {
		return ""
	}
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	return task.Properties.UpstreamModelName
}

func dashboardQuotaDataKey(userId int, username string, modelName string, createdAt int64) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%d", userId, username, modelName, createdAt)
}

func applyDashboardLogFilters(tx *gorm.DB, userId int, username string, startTime int64, endTime int64) (*gorm.DB, error) {
	if userId != 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	if username != "" {
		var err error
		tx, err = applyExplicitLogTextFilter(tx, "username", username)
		if err != nil {
			return nil, err
		}
	}
	if startTime != 0 {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime != 0 {
		tx = tx.Where("created_at <= ?", endTime)
	}
	return tx, nil
}

// GetActualQuotaData returns settlement-based dashboard data.
//
// Quota is always net consumption: consume - refund. Count is successful
// request count: successful tasks plus successful synchronous consume logs.
// Pending or failed async tasks are deliberately excluded from Count.
func GetActualQuotaData(userId int, username string, startTime int64, endTime int64) ([]*QuotaData, error) {
	dataMap := make(map[string]*QuotaData)
	getData := func(userId int, username string, modelName string, createdAt int64) *QuotaData {
		createdAt -= createdAt % 3600
		key := dashboardQuotaDataKey(userId, username, modelName, createdAt)
		if item, ok := dataMap[key]; ok {
			return item
		}
		item := &QuotaData{
			UserID:    userId,
			Username:  username,
			ModelName: modelName,
			CreatedAt: createdAt,
		}
		dataMap[key] = item
		return item
	}

	logQuery := LOG_DB.Table("logs").Select(
		"user_id, username, model_name, created_at - (created_at % 3600) AS created_at, " +
			"SUM(CASE WHEN type = ? THEN quota ELSE -quota END) AS quota, " +
			"SUM(CASE WHEN type = ? AND (other IS NULL OR other = '' OR (other NOT LIKE ? AND other NOT LIKE ?)) " +
				"THEN prompt_tokens + completion_tokens ELSE 0 END) AS token_used",
		LogTypeConsume, LogTypeConsume, dashboardTaskFlagPattern, dashboardTaskIDPattern,
	).Where("type IN ?", []int{LogTypeConsume, LogTypeRefund})
	var err error
	logQuery, err = applyDashboardLogFilters(logQuery, userId, username, startTime, endTime)
	if err != nil {
		return nil, err
	}
	var quotaRows []dashboardAggregateRow
	if err := logQuery.Group("user_id, username, model_name, created_at - (created_at % 3600)").Find(&quotaRows).Error; err != nil {
		return nil, err
	}
	for _, row := range quotaRows {
		item := getData(row.UserID, row.Username, row.ModelName, row.CreatedAt)
		item.Quota += row.Quota
		item.TokenUsed += row.TokenUsed
	}

	// Synchronous success logs contribute call counts. Task logs are excluded
	// here because async calls are counted from successful tasks below.
	syncQuery := LOG_DB.Table("logs").Select(
		"user_id, username, model_name, created_at - (created_at % 3600) AS created_at, COUNT(*) AS count",
	).Where("type = ?", LogTypeConsume).
		Where("(other IS NULL OR other = '' OR (other NOT LIKE ? AND other NOT LIKE ?))", dashboardTaskFlagPattern, dashboardTaskIDPattern).
		Where("(quota > 0 OR prompt_tokens + completion_tokens > 0)")
	syncQuery, err = applyDashboardLogFilters(syncQuery, userId, username, startTime, endTime)
	if err != nil {
		return nil, err
	}
	var syncRows []dashboardAggregateRow
	if err := syncQuery.Group("user_id, username, model_name, created_at - (created_at % 3600)").Find(&syncRows).Error; err != nil {
		return nil, err
	}
	for _, row := range syncRows {
		item := getData(row.UserID, row.Username, row.ModelName, row.CreatedAt)
		item.Count += row.Count
	}

	taskQuery := DB.Model(&Task{}).Select("user_id, submit_time, created_at, properties").Where("status = ?", TaskStatusSuccess)
	if userId != 0 {
		taskQuery = taskQuery.Where("user_id = ?", userId)
	}
	if username != "" {
		userIds, err := GetUserIdsByUsernameFilter(username)
		if err != nil {
			return nil, err
		}
		if len(userIds) == 0 {
			taskQuery = taskQuery.Where("1 = 0")
		} else {
			taskQuery = taskQuery.Where("user_id IN ?", userIds)
		}
	}
	if startTime != 0 {
		taskQuery = taskQuery.Where("submit_time >= ?", startTime)
	}
	if endTime != 0 {
		taskQuery = taskQuery.Where("submit_time <= ?", endTime)
	}

	var successfulTasks []Task
	if err := taskQuery.Find(&successfulTasks).Error; err != nil {
		return nil, err
	}
	usernamesByID := make(map[int]string)
	if len(successfulTasks) > 0 {
		userIDs := make([]int, 0, len(successfulTasks))
		seenUserIDs := make(map[int]struct{})
		for i := range successfulTasks {
			if _, ok := seenUserIDs[successfulTasks[i].UserId]; ok {
				continue
			}
			seenUserIDs[successfulTasks[i].UserId] = struct{}{}
			userIDs = append(userIDs, successfulTasks[i].UserId)
		}
		var users []User
		if err := DB.Select("id, username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		for i := range users {
			usernamesByID[users[i].Id] = users[i].Username
		}
	}
	for i := range successfulTasks {
		task := &successfulTasks[i]
		createdAt := task.SubmitTime
		if createdAt == 0 {
			createdAt = task.CreatedAt
		}
		taskUsername := usernamesByID[task.UserId]
		item := getData(task.UserId, taskUsername, dashboardTaskModelName(task), createdAt)
		item.Count++
	}

	result := make([]*QuotaData, 0, len(dataMap))
	for _, item := range dataMap {
		if item.Count == 0 && item.Quota == 0 && item.TokenUsed == 0 {
			continue
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt == result[j].CreatedAt {
			if result[i].ModelName == result[j].ModelName {
				return result[i].Username < result[j].Username
			}
			return result[i].ModelName < result[j].ModelName
		}
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result, nil
}
