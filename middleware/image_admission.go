package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	imageSyncLease        = 90 * time.Second
	imageSyncRenewal      = 20 * time.Second
	imageRequestPeekBytes = 1 << 20
	imageSyncRedisKey     = "newapi:image_sync:active:v1"
)

// Redis time keeps admission consistent even when gateway hosts have different clocks.
const imageSyncAcquireScript = `
local now = redis.call('TIME')
local milliseconds = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', milliseconds)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then return 0 end
redis.call('ZADD', KEYS[1], milliseconds + tonumber(ARGV[3]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3]) * 2)
return 1`

const imageSyncRenewScript = `
if not redis.call('ZSCORE', KEYS[1], ARGV[1]) then return 0 end
local now = redis.call('TIME')
local milliseconds = now[1] * 1000 + math.floor(now[2] / 1000)
redis.call('ZADD', KEYS[1], milliseconds + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]) * 2)
return 1`

const imageSyncReleaseScript = `return redis.call('ZREM', KEYS[1], ARGV[1])`

var imageSyncLocalInFlight atomic.Int64

// ImageSyncAdmission protects the legacy synchronous image endpoints before
// Distribute reads their body. Background Responses image jobs do not hold a slot.
func ImageSyncAdmission() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isSynchronousImageRequest(c) {
			c.Next()
			return
		}

		limit := common.GetEnvOrDefault("IMAGE_SYNC_MAX_CONCURRENT", 1)
		if limit < 1 {
			limit = 1
		}
		owner := uuid.NewString()
		release, renew, err := acquireImageSyncSlot(c.Request.Context(), owner, limit)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
				"type": "service_unavailable", "message": "Image admission is temporarily unavailable",
			}})
			return
		}
		if release == nil {
			c.Header("Retry-After", "2")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{
				"type": "rate_limit_error", "message": "Synchronous image capacity is busy; use background image generation or retry",
			}})
			return
		}
		defer release()
		maxRequestMB := common.GetEnvOrDefault("IMAGE_SYNC_MAX_REQUEST_MB", 32)
		if maxRequestMB < 1 || maxRequestMB > 128 {
			maxRequestMB = 32
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxRequestMB)<<20)

		requestContext, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		c.Request = c.Request.WithContext(requestContext)
		if renew != nil {
			go func() {
				ticker := time.NewTicker(imageSyncRenewal)
				defer ticker.Stop()
				for {
					select {
					case <-requestContext.Done():
						return
					case <-ticker.C:
						if !renew(requestContext) {
							cancel()
							return
						}
					}
				}
			}()
		}
		c.Next()
	}
}

func isSynchronousImageRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost {
		return false
	}
	switch c.Request.URL.Path {
	case "/v1/images/generations", "/v1/images/edits", "/v1/edits":
		return true
	case "/v1/responses", "/v1/chat/completions":
		// Large or unknown-size Responses bodies take the conservative path. The
		// normal URL-based background request is much smaller than this bound.
		if c.Request.ContentLength > imageRequestPeekBytes {
			return true
		}
		original := c.Request.Body
		if original == nil {
			return false
		}
		prefix, err := io.ReadAll(io.LimitReader(original, imageRequestPeekBytes+1))
		c.Request.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(prefix), original), Closer: original}
		if err != nil || len(prefix) > imageRequestPeekBytes {
			return true
		}
		var request struct {
			Model      string `json:"model"`
			Background bool   `json:"background"`
			Stream     bool   `json:"stream"`
			Tools      []struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice json.RawMessage `json:"tool_choice"`
		}
		if err := common.Unmarshal(prefix, &request); err != nil {
			// Distribute reports malformed JSON. Reserve a slot until it does.
			return true
		}
		var choice struct {
			Type string `json:"type"`
		}
		if len(request.ToolChoice) > 0 && request.ToolChoice[0] == '{' {
			_ = common.Unmarshal(request.ToolChoice, &choice)
		}
		imageTool := strings.EqualFold(choice.Type, "image_generation") ||
			strings.EqualFold(choice.Type, "image_generation_call")
		if len(request.ToolChoice) > 0 && request.ToolChoice[0] == '"' {
			var toolChoice string
			_ = common.Unmarshal(request.ToolChoice, &toolChoice)
			imageTool = imageTool || strings.EqualFold(toolChoice, "image_generation") ||
				strings.EqualFold(toolChoice, "image_generation_call")
		}
		for _, tool := range request.Tools {
			if strings.EqualFold(tool.Type, "image_generation") || strings.EqualFold(tool.Type, "image_generation_call") ||
				strings.EqualFold(tool.Function.Name, "image_generation") {
				imageTool = true
				break
			}
		}
		imageModel := isImageModelName(request.Model)
		if c.Request.URL.Path == "/v1/chat/completions" {
			return imageTool || imageModel
		}
		return (imageTool || imageModel) && (!request.Background || request.Stream)
	default:
		return false
	}
}

func isImageModelName(modelName string) bool {
	modelName = strings.ToLower(modelName)
	return strings.Contains(modelName, "gpt-image") || strings.Contains(modelName, "nano-banana") ||
		strings.Contains(modelName, "seedream") || strings.Contains(modelName, "flux-image")
}

func acquireImageSyncSlot(ctx context.Context, owner string, limit int) (func(), func(context.Context) bool, error) {
	if !common.RedisEnabled || common.RDB == nil {
		for {
			current := imageSyncLocalInFlight.Load()
			if current >= int64(limit) {
				return nil, nil, nil
			}
			if imageSyncLocalInFlight.CompareAndSwap(current, current+1) {
				return func() { imageSyncLocalInFlight.Add(-1) }, nil, nil
			}
		}
	}
	result, err := common.RDB.Eval(ctx, imageSyncAcquireScript, []string{imageSyncRedisKey}, owner, limit, imageSyncLease.Milliseconds()).Int()
	if err != nil || result == 0 {
		return nil, nil, err
	}
	release := func() {
		releaseContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = common.RDB.Eval(releaseContext, imageSyncReleaseScript, []string{imageSyncRedisKey}, owner).Err()
	}
	renew := func(ctx context.Context) bool {
		refreshContext, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		value, err := common.RDB.Eval(refreshContext, imageSyncRenewScript, []string{imageSyncRedisKey}, owner, imageSyncLease.Milliseconds()).Int()
		return err == nil && value == 1
	}
	return release, renew, nil
}
