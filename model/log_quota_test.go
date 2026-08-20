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
}
