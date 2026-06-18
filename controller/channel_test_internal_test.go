package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestNormalizeChannelTestEndpointForcesVideoModelsToVideoEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		channel      *model.Channel
		modelName    string
		endpointType string
	}{
		{
			name:         "video 2 model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "video-2.0",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "video 2 fast model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "video-2.0-fast",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "sora2 model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "sora2",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "ko3 model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "ko3",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "veo31 model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "veo31",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "kling v3 model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "kling-v3",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "grok imagine video model overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeOpenAI},
			modelName:    "grok-imagine-video",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
		{
			name:         "sora channel overrides explicit openai endpoint",
			channel:      &model.Channel{Type: constant.ChannelTypeSora},
			modelName:    "custom-video-model",
			endpointType: string(constant.EndpointTypeOpenAI),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpointType := normalizeChannelTestEndpoint(test.channel, test.modelName, test.endpointType)

			require.Equal(t, string(constant.EndpointTypeOpenAIVideo), endpointType)
		})
	}
}

func TestNormalizeChannelTestEndpointKeepsExplicitEndpointForNonVideoModel(t *testing.T) {
	endpointType := normalizeChannelTestEndpoint(
		&model.Channel{Type: constant.ChannelTypeOpenAI},
		"gpt-4o",
		string(constant.EndpointTypeOpenAIResponse),
	)

	require.Equal(t, string(constant.EndpointTypeOpenAIResponse), endpointType)
}
