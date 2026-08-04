package hailuo

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertToMiniMaxH3RequestPayloadDefaultAspectRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH3,
		Prompt: "龟兔赛跑",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH3}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, ModelMiniMaxH3, body.Model)
	require.Equal(t, "龟兔赛跑", body.Prompt)
	require.NotNil(t, body.Duration)
	require.Equal(t, 5, *body.Duration)
	require.Equal(t, 2560, body.Width)
	require.Equal(t, 1440, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadSizeOverridesAspectRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:       ModelMiniMaxH3,
		Prompt:      "珠宝广告",
		Duration:    8,
		AspectRatio: "16:9",
		Size:        "1440x2560",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH3}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, 8, *body.Duration)
	require.Equal(t, 1440, body.Width)
	require.Equal(t, 2560, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadUpscalesCommonSize(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH3,
		Prompt: "山间云海",
		Size:   "1280x720",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH3}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, 2560, body.Width)
	require.Equal(t, 1440, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadImageReferences(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:    ModelMiniMaxH3,
		Prompt:   "图一和图二的角色出现在图三的场景中",
		Duration: 8,
		Images: []string{
			"https://example.com/character-1.png",
			"https://example.com/character-2.png",
			"https://example.com/scene.png",
		},
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH3}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Empty(t, body.ImageURL)
	require.Equal(t, req.Images, body.ImageURLs)
}

func TestConvertToMiniMaxH3RequestPayloadRejectsInvalidCombinations(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH3}

	_, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:    ModelMiniMaxH3,
		Prompt:   "恐龙冒险",
		AudioURL: "https://example.com/adventure.mp3",
	}, info)
	require.ErrorContains(t, err, "audio_url requires")

	_, err = adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:         ModelMiniMaxH3,
		Prompt:        "恐龙逐渐变成兔子",
		StartImageURL: "https://example.com/start.png",
		Images:        []string{"https://example.com/ref.png"},
	}, info)
	require.ErrorContains(t, err, "cannot be mixed")
}
