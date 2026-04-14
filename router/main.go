package router

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func retryWithoutTrailingSlash(router *gin.Engine, c *gin.Context) bool {
	path := c.Request.URL.Path
	if path == "/" || !strings.HasSuffix(path, "/") {
		return false
	}
	trimmedPath := strings.TrimRight(path, "/")
	if trimmedPath == "" {
		trimmedPath = "/"
	}
	rawPath := c.Request.URL.RawPath
	trimmedRawPath := strings.TrimRight(rawPath, "/")
	rawQuery := c.Request.URL.RawQuery

	c.Request.URL.Path = trimmedPath
	if rawPath != "" {
		c.Request.URL.RawPath = trimmedRawPath
	}
	c.Request.RequestURI = trimmedPath
	if rawQuery != "" {
		c.Request.RequestURI += "?" + rawQuery
	}
	router.HandleContext(c)
	return true
}

func SetRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetVideoRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, buildFS, indexPage)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			if retryWithoutTrailingSlash(router, c) {
				return
			}
			c.Set(middleware.RouteTagKey, "web")
			c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
		})
	}
}
