package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestNormalizeLinkSkyAsyncVideoBodyUsesNumericDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":   "sora-2",
		"seconds": "4",
		"size":    "1280x720",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, "sora2", body["model"])
	require.Equal(t, 4, body["duration"])
	require.Equal(t, "16:9", body["aspect_ratio"])
	require.Equal(t, true, body["async"])
}

func TestNormalizeLinkSkyAsyncVideoBodyCoercesExistingDuration(t *testing.T) {
	body := map[string]interface{}{
		"model":    "video-2.0",
		"duration": "10",
	}

	normalizeLinkSkyAsyncVideoBody(body)

	require.Equal(t, 10, body["duration"])
}

func TestBuildRequestURLUsesAsyncEndpointForLinkSkyOpenAIChannel(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://linksky.top",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://linksky.top/v1/video/async-generations", url)
}

func TestBuildRequestURLKeepsOpenAIVideoEndpointForOtherOpenAIChannels(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://api.openai.com",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/videos", url)
}

func TestBuildRequestURLUsesAsyncEndpointForLeoGoModelAlias(t *testing.T) {
	adaptor := &TaskAdaptor{
		ChannelType: constant.ChannelTypeOpenAI,
		baseURL:     "https://example-leogo.test",
	}

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "video-2.0-fast"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NoError(t, err)
	require.Equal(t, "https://example-leogo.test/v1/video/async-generations", url)
}
