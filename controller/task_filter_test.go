package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func taskFilterTestContext(query string) *gin.Context {
	request := httptest.NewRequest("GET", "/api/task/?"+query, nil)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}

func TestAdminTaskUserFilterAcceptsUsernameOrID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID, username := adminTaskUserFilter(taskFilterTestContext("user=42"))
	require.Equal(t, "42", userID)
	require.Empty(t, username)

	userID, username = adminTaskUserFilter(
		taskFilterTestContext("user=alpha"),
	)
	require.Empty(t, userID)
	require.Equal(t, "alpha", username)
}

func TestAdminTaskUserFilterSupportsExplicitLegacyParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID, username := adminTaskUserFilter(
		taskFilterTestContext("user_id=7&username=alpha"),
	)
	require.Equal(t, "7", userID)
	require.Equal(t, "alpha", username)
}

func TestTaskModelFilterSupportsBothParameterNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	require.Equal(
		t,
		"minimax-h3",
		taskModelFilter(taskFilterTestContext("model_name=minimax-h3")),
	)
	require.Equal(
		t,
		"hailuo-2.3",
		taskModelFilter(taskFilterTestContext("model=hailuo-2.3")),
	)
}
