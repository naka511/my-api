package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaSubtractsRefunds(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create([]*Log{
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     100,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeRefund,
			ModelName: "video-2.0",
			Quota:     100,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     70,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeTopup,
			ModelName: "video-2.0",
			Quota:     55,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeManage,
			ModelName: "video-2.0",
			Quota:     -20,
		},
	}).Error)

	stat, err := SumUsedQuota(0, now-1, now+1, "video-2.0", "test-user", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 70, stat.Quota)

	stat, err = SumUsedQuota(LogTypeConsume, now-1, now+1, "video-2.0", "test-user", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 170, stat.Quota)

	stat, err = SumUsedQuota(LogTypeRefund, now-1, now+1, "video-2.0", "test-user", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 100, stat.Quota)

	stat, err = SumUsedQuota(LogTypeTopup, now-1, now+1, "video-2.0", "test-user", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 55, stat.Quota)

	stat, err = SumUsedQuota(LogTypeManage, now-1, now+1, "video-2.0", "test-user", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, -20, stat.Quota)
}

func TestSumUsageLogQuotaUsesOnlyFilteredLogs(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create([]*Log{
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     120,
			RequestId: "request-a",
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeRefund,
			ModelName: "video-2.0",
			Quota:     120,
			RequestId: "request-a",
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     80,
			RequestId: "request-b",
		},
	}).Error)

	stat, err := SumUsageLogQuota(0, now-1, now+1, "video-2.0", "test-user", "", 0, "", "request-a", "")
	require.NoError(t, err)
	require.Equal(t, 0, stat.Quota)

	stat, err = SumUsageLogQuota(0, now-1, now+1, "video-2.0", "test-user", "", 0, "", "request-b", "")
	require.NoError(t, err)
	require.Equal(t, 80, stat.Quota)
}

func TestGetActualQuotaDataCountsOnlySuccessfulTasks(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&User{Id: 1, Username: "test-user"}).Error)
	require.NoError(t, DB.Create([]*Task{
		{
			TaskID:    "task-success",
			UserId:    1,
			Quota:     100,
			SubmitTime: now,
			Status:    TaskStatusSuccess,
			Properties: Properties{
				OriginModelName: "video-2.0",
			},
		},
		{
			TaskID:    "task-failure",
			UserId:    1,
			SubmitTime: now,
			Status:    TaskStatusFailure,
			Properties: Properties{
				OriginModelName: "video-2.0",
			},
		},
	}).Error)
	require.NoError(t, LOG_DB.Create([]*Log{
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     100,
			Other:     `{"is_task":true,"task_id":"task-success"}`,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     80,
			Other:     `{"is_task":true,"task_id":"task-failure"}`,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeRefund,
			ModelName: "video-2.0",
			Quota:     80,
			Other:     `{"is_task":true,"task_id":"task-failure"}`,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     50,
			Other:     `{"request_path":"/v1/chat/completions"}`,
		},
	}).Error)

	data, err := GetActualQuotaData(1, "", now-1, now+1)
	require.NoError(t, err)
	require.Len(t, data, 1)
	require.Equal(t, 150, data[0].Quota)
	require.Equal(t, 2, data[0].Count)
}

func TestGetActualQuotaDataAttributesTaskSettlementToSubmitHour(t *testing.T) {
	truncateTables(t)
	now := time.Now().Truncate(time.Hour).Unix()
	require.NoError(t, DB.Create(&User{Id: 1, Username: "test-user"}).Error)
	require.NoError(t, DB.Create([]*Task{
		{
			TaskID:     "task-success-late-settlement",
			UserId:     1,
			Quota:      70,
			SubmitTime: now - 3600,
			Status:     TaskStatusSuccess,
			Properties: Properties{OriginModelName: "video-2.0"},
		},
		{
			TaskID:     "task-failure-late-refund",
			UserId:     1,
			Quota:      80,
			SubmitTime: now - 3600,
			Status:     TaskStatusFailure,
			Properties: Properties{OriginModelName: "video-2.0"},
		},
	}).Error)
	require.NoError(t, LOG_DB.Create([]*Log{
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now + 1,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     70,
			Other:     `{"is_task":true,"task_id":"task-success-late-settlement"}`,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now + 2,
			Type:      LogTypeConsume,
			ModelName: "video-2.0",
			Quota:     80,
			Other:     `{"is_task":true,"task_id":"task-failure-late-refund"}`,
		},
		{
			UserId:    1,
			Username:  "test-user",
			CreatedAt: now + 3,
			Type:      LogTypeRefund,
			ModelName: "video-2.0",
			Quota:     80,
			Other:     `{"is_task":true,"task_id":"task-failure-late-refund"}`,
		},
	}).Error)

	currentHour, err := GetActualQuotaData(1, "", now, now+3599)
	require.NoError(t, err)
	require.Empty(t, currentHour)

	submitHour, err := GetActualQuotaData(1, "", now-3600, now-1)
	require.NoError(t, err)
	require.Len(t, submitHour, 1)
	require.Equal(t, 70, submitHour[0].Quota)
	require.Equal(t, 1, submitHour[0].Count)
}
