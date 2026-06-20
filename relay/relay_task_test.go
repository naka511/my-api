package relay

import (
	"testing"

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
