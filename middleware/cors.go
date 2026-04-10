package middleware

import (
	"net/url"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func buildAllowedOrigins() map[string]struct{} {
	allowed := map[string]struct{}{}

	// Explicit env override, comma-separated.
	if raw := strings.TrimSpace(os.Getenv("CORS_ALLOW_ORIGINS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			origin := strings.TrimSpace(part)
			if origin == "" {
				continue
			}
			allowed[origin] = struct{}{}
		}
	}

	// Reuse system setting server address as an allowed origin when set.
	common.OptionMapRWMutex.RLock()
	serverAddress := strings.TrimSpace(common.OptionMap["ServerAddress"])
	common.OptionMapRWMutex.RUnlock()
	if serverAddress != "" {
		if u, err := url.Parse(serverAddress); err == nil && u.Scheme != "" && u.Host != "" {
			allowed[u.Scheme+"://"+u.Host] = struct{}{}
		}
	}

	// Common local dev origins.
	allowed["http://localhost:3000"] = struct{}{}
	allowed["http://localhost:5173"] = struct{}{}
	allowed["http://127.0.0.1:3000"] = struct{}{}
	allowed["http://127.0.0.1:5173"] = struct{}{}

	return allowed
}

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "New-API-User", "X-Requested-With"}
	allowedOrigins := buildAllowedOrigins()
	config.AllowOriginFunc = func(origin string) bool {
		// Non-browser/server-to-server requests may not carry Origin.
		if origin == "" {
			return true
		}
		if _, ok := allowedOrigins[origin]; ok {
			return true
		}
		return false
	}
	return cors.New(config)
}

func PoweredBy() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
