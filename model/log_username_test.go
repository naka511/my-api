package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveLogUsernameUsesContextKeyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	common.SetContextKey(ctx, constant.ContextKeyUserName, "token-user")

	require.Equal(t, "token-user", resolveLogUsername(ctx, 123))
}

func TestUserBaseWriteContextSetsGinUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}

	(&UserBase{Username: "token-user"}).WriteContext(ctx)

	require.Equal(t, "token-user", ctx.GetString("username"))
	require.Equal(t, "token-user", common.GetContextKeyString(ctx, constant.ContextKeyUserName))
}
