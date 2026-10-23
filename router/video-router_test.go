package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetVideoRouterRegistersOfficialVideoRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	SetVideoRouter(r)

	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	require.True(t, routes[http.MethodPost+" /api/v1/services/aigc/video-generation/video-synthesis"])
	require.True(t, routes[http.MethodGet+" /api/v1/tasks/:task_id"])
	require.True(t, routes[http.MethodPost+" /api/v3/contents/generations/tasks"])
	require.True(t, routes[http.MethodGet+" /api/v3/contents/generations/tasks/:task_id"])
	require.True(t, routes[http.MethodPost+" /oapi/v3/contents/generations/tasks"])
	require.True(t, routes[http.MethodGet+" /oapi/v3/contents/generations/tasks/:task_id"])
	require.True(t, routes[http.MethodPost+" /text-to-video/kling-4.0"])
	require.True(t, routes[http.MethodPost+" /image-to-video/kling-4.0"])
	require.True(t, routes[http.MethodPost+" /omni-video/kling-4.0"])
	require.True(t, routes[http.MethodGet+" /tasks"])
	require.True(t, routes[http.MethodPost+" /kling/v1/videos/text2video"])
	require.True(t, routes[http.MethodPost+" /v1/videos"])
	require.False(t, routes[http.MethodPost+" /tasks"])
}

func TestKlingNativeRoutesValidateAndAuthenticateWithoutGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetVideoRouter(r)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/text-to-video/kling-4.0", `{"prompt":"test","settings":{"duration":3}}`, 400},
		{http.MethodPost, "/text-to-video/kling-4.0", `{"prompt":"test"}`, 401},
		{http.MethodPost, "/image-to-video/kling-4.0", `{"contents":[{"type":"prompt","text":"test"},{"type":"first_frame","url":"https://example.com/x"}]}`, 401},
		{http.MethodPost, "/omni-video/kling-4.0", `{"contents":[{"type":"prompt","text":"test"},{"type":"refer_image","url":"https://example.com/x"}]}`, 401},
		{http.MethodGet, "/tasks?task_ids=task_private", "", 401},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(recorder, request)
		require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
		var response map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(t, float64(tc.status), response["code"])
		require.Nil(t, response["data"])
	}
}
