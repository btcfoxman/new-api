package router

import (
	"net/http"
	"testing"

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
}
