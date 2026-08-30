package hailuo

import (
	"fmt"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertToMiniMaxH3RequestPayloadDefaultAspectRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH32K,
		Prompt: "龟兔赛跑",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, UpstreamModelMiniMaxH3, body.Model)
	require.Equal(t, "龟兔赛跑", body.Prompt)
	require.NotNil(t, body.Duration)
	require.Equal(t, 5, *body.Duration)
	require.Equal(t, 2560, body.Width)
	require.Equal(t, 1440, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadUsesVariantSize(t *testing.T) {
	tests := []struct {
		model  string
		width  int
		height int
	}{
		{model: ModelMiniMaxH3480P, width: 856, height: 480},
		{model: ModelMiniMaxH3768P, width: 1376, height: 768},
		{model: ModelMiniMaxH32K, width: 2560, height: 1440},
		{model: ModelMiniMaxH34K, width: 3840, height: 2160},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			adaptor := &TaskAdaptor{}
			body, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
				Model:  test.model,
				Prompt: "variant size",
			}, &relaycommon.RelayInfo{UpstreamModelName: test.model})

			require.NoError(t, err)
			require.Equal(t, UpstreamModelMiniMaxH3, body.Model)
			require.Equal(t, test.width, body.Width)
			require.Equal(t, test.height, body.Height)
		})
	}
}

func TestConvertToMiniMaxH3RequestPayloadRejectsLegacyModel(t *testing.T) {
	adaptor := &TaskAdaptor{}
	_, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  UpstreamModelMiniMaxH3,
		Prompt: "legacy model",
	}, &relaycommon.RelayInfo{UpstreamModelName: UpstreamModelMiniMaxH3})

	require.ErrorContains(t, err, "no longer available")
}

func TestConvertToMiniMaxH3RequestPayloadSizeOverridesAspectRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:       ModelMiniMaxH32K,
		Prompt:      "珠宝广告",
		Duration:    8,
		AspectRatio: "16:9",
		Size:        "1440x2560",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, 8, *body.Duration)
	require.Equal(t, 1440, body.Width)
	require.Equal(t, 2560, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadUsesModelSize(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH32K,
		Prompt: "山间云海",
		Size:   "2560x1440",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Equal(t, 2560, body.Width)
	require.Equal(t, 1440, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadImageReferences(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:    ModelMiniMaxH32K,
		Prompt:   "图一和图二的角色出现在图三的场景中",
		Duration: 8,
		Images: []string{
			"https://example.com/character-1.png",
			"https://example.com/character-2.png",
			"https://example.com/scene.png",
		},
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Empty(t, body.ImageURL)
	require.Equal(t, req.Images, body.ImageURLs)
}

func TestConvertToMiniMaxH3RequestPayloadSupportsImageURLs(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:      ModelMiniMaxH32K,
		Prompt:     "多图参考",
		ImageURLs:  []string{"https://example.com/a.png", "https://example.com/b.png"},
		AspectRatio: "9:16",
	}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(req, info)

	require.NoError(t, err)
	require.Empty(t, body.ImageURL)
	require.Equal(t, req.ImageURLs, body.ImageURLs)
	require.Equal(t, 1440, body.Width)
	require.Equal(t, 2560, body.Height)
}

func TestConvertToMiniMaxH3RequestPayloadRejectsInvalidDurationAndPrompt(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	_, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:    ModelMiniMaxH32K,
		Prompt:   "短视频",
		Duration: 4,
	}, info)
	require.ErrorContains(t, err, "duration must be between 5 and 15 seconds")

	_, err = adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH32K,
		Prompt: string(make([]rune, 5001)),
	}, info)
	require.ErrorContains(t, err, "prompt must not exceed 5000 characters")
}

func TestConvertToMiniMaxH3RequestPayloadRequiresBothFrameImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	_, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:         ModelMiniMaxH32K,
		Prompt:        "首尾帧",
		StartImageURL: "https://example.com/start.png",
	}, info)
	require.ErrorContains(t, err, "must be provided together")
}

func TestConvertToMiniMaxH3RequestPayloadRejectsInvalidCombinations(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K}

	body, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:    ModelMiniMaxH32K,
		Prompt:   "恐龙冒险",
		AudioURL: "https://example.com/adventure.mp3",
	}, info)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/adventure.mp3", body.AudioURL)

	_, err = adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:         ModelMiniMaxH32K,
		Prompt:        "恐龙逐渐变成兔子",
		StartImageURL: "https://example.com/start.png",
		Images:        []string{"https://example.com/ref.png"},
	}, info)
	require.ErrorContains(t, err, "cannot be mixed")
}

func TestConvertToMiniMaxH3RequestPayloadSupportsDocumentedReferences(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH32K,
		Prompt: "多模态参考",
		ImageGuidance: []relaycommon.TaskImageGuidance{{
			URL:      "https://example.com/guide.png",
			Strength: 0.8,
		}},
		VideoReference: []relaycommon.TaskMediaReference{{
			URL:      "https://example.com/motion.mp4",
			Duration: 6,
		}},
		AudioReference: []relaycommon.TaskMediaReference{{
			URL:      "https://example.com/music.mp3",
			Duration: 12,
		}},
	}

	body, err := adaptor.convertToRequestPayload(req, &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K})

	require.NoError(t, err)
	require.Equal(t, req.ImageGuidance[0].URL, body.ImageGuidance[0].URL)
	require.Equal(t, req.ImageGuidance[0].Strength, body.ImageGuidance[0].Strength)
	require.Equal(t, req.VideoReference[0].URL, body.VideoReference[0].URL)
	require.Equal(t, req.VideoReference[0].Duration, body.VideoReference[0].Duration)
	require.Equal(t, req.AudioReference[0].URL, body.AudioReference[0].URL)
	require.Equal(t, req.AudioReference[0].Duration, body.AudioReference[0].Duration)
}

func TestConvertToMiniMaxH3RequestPayloadSupportsNineImageReferences(t *testing.T) {
	adaptor := &TaskAdaptor{}
	images := make([]string, 9)
	for i := range images {
		images[i] = fmt.Sprintf("https://example.com/image-%d.png", i)
	}

	body, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:  ModelMiniMaxH32K,
		Prompt: "九张参考图",
		Images: images,
	}, &relaycommon.RelayInfo{UpstreamModelName: ModelMiniMaxH32K})

	require.NoError(t, err)
	require.Equal(t, images, body.ImageURLs)
}
