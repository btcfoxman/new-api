package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorReturnsHappyHorseCompatibleFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", nil)
	c.Set(common.RequestIdKey, "request_123")

	respondTaskError(c, &dto.TaskError{
		Code:       "invalid_request",
		Message:    "prompt is required",
		Data:       nil,
		StatusCode: http.StatusBadRequest,
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "invalid_request", payload["code"])
	require.Equal(t, "prompt is required", payload["message"])
	require.Equal(t, "request_123", payload["request_id"])
	errorData, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request", errorData["code"])
	require.Equal(t, "prompt is required", errorData["message"])
	require.Equal(t, "invalid_request_error", errorData["type"])
}
