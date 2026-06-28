package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestShouldApplyTaskOtherRatiosKeepsLegacyPerRequestPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			UsePrice: true,
		},
	}

	require.True(t, shouldApplyTaskOtherRatios(info, "video-2.0-fast"))
}

func TestShouldApplyTaskOtherRatiosKeepsUsageRatioBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			UsePrice: false,
		},
	}

	require.True(t, shouldApplyTaskOtherRatios(info, "video-2.0-fast"))
}

func TestShouldApplyTaskOtherRatiosSkipsFixedPriceMode(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"video-2.0-fast":"fixed_price"}`,
	}))

	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			UsePrice: true,
		},
	}

	require.False(t, shouldApplyTaskOtherRatios(info, "video-2.0-fast"))
}

func TestBuildVideo2AsyncTaskResponseHidesInternalFields(t *testing.T) {
	task := &model.Task{
		ID:         123,
		TaskID:     "task_public",
		UserId:     10,
		ChannelId:  15,
		Quota:      1100000,
		Status:     model.TaskStatusInProgress,
		SubmitTime: 1782662469,
		Progress:   "30%",
		Properties: model.Properties{
			OriginModelName: "video-2.0",
		},
	}

	resp := buildVideo2AsyncTaskResponse(task)
	require.Equal(t, "task_public", resp.TaskID)
	require.Equal(t, "video", resp.Object)
	require.Equal(t, "video-2.0", resp.Model)
	require.Equal(t, "processing", resp.Status)
	require.Equal(t, 30, resp.Progress)

	body, err := common.Marshal(resp)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.NotContains(t, payload, "user_id")
	require.NotContains(t, payload, "channel_id")
	require.NotContains(t, payload, "quota")
	require.NotContains(t, payload, "properties")
	require.NotContains(t, payload, "data")
}
