package middleware

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// KlingNativeRequestConvert validates the supported Kling 4.0 subset before
// channel selection or billing, then uses the existing video relay pipeline.
func KlingNativeRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request map[string]any
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "invalid JSON request"})
			return
		}
		normalized, err := normalizeKlingNativeRequest(c.Request.URL.Path, request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		body, err := common.Marshal(normalized)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "failed to encode request"})
			return
		}
		common.CleanupBodyStorage(c)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.URL.Path = "/v1/video/generations"
		c.Set(common.KeyRequestBody, body)
		c.Next()
	}
}

func klingNativeKeys(value map[string]any, prefix string, allowed ...string) error {
	for key := range value {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unsupported parameter: %s%s", prefix, key)
		}
	}
	return nil
}

func klingNativeObject(request map[string]any, key string) (map[string]any, error) {
	value, exists := request[key]
	if !exists {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return object, nil
}

func normalizeKlingNativeRequest(path string, request map[string]any) (map[string]any, error) {
	textOnly := path == "/text-to-video/kling-4.0"
	imageOnly := path == "/image-to-video/kling-4.0"
	if !textOnly && !imageOnly && path != "/omni-video/kling-4.0" {
		return nil, fmt.Errorf("unsupported Kling endpoint")
	}
	keys := []string{"contents", "settings", "options"}
	if textOnly {
		keys = append(keys, "prompt")
	}
	if err := klingNativeKeys(request, "", keys...); err != nil {
		return nil, err
	}
	settings, err := klingNativeObject(request, "settings")
	if err != nil {
		return nil, err
	}
	keys = []string{"resolution", "duration"}
	if !imageOnly {
		keys = append(keys, "aspect_ratio")
	}
	if err := klingNativeKeys(settings, "settings.", keys...); err != nil {
		return nil, err
	}
	resolution := "1080p"
	if value, exists := settings["resolution"]; exists {
		resolution, _ = value.(string)
		if resolution != "720p" && resolution != "1080p" {
			return nil, fmt.Errorf("settings.resolution must be 720p or 1080p")
		}
	}
	duration := 10
	if value, exists := settings["duration"]; exists {
		switch typed := value.(type) {
		case float64:
			if math.Trunc(typed) != typed || typed < 4 || typed > 30 {
				return nil, fmt.Errorf("settings.duration supports integer seconds from 4 to 30")
			}
			duration = int(typed)
		case string:
			duration, err = strconv.Atoi(typed)
		default:
			err = fmt.Errorf("invalid duration")
		}
		if err != nil || duration < 4 || duration > 30 {
			return nil, fmt.Errorf("settings.duration supports integer seconds from 4 to 30")
		}
	}
	ratio := "16:9"
	if value, exists := settings["aspect_ratio"]; exists {
		ratio, _ = value.(string)
		switch ratio {
		case "21:9", "16:9", "9:16", "1:1":
		default:
			return nil, fmt.Errorf("unsupported settings.aspect_ratio; use 21:9, 16:9, 9:16 or 1:1")
		}
	}
	options, err := klingNativeObject(request, "options")
	if err != nil {
		return nil, err
	}
	if err := klingNativeKeys(options, "options.", "callback_url", "external_task_id", "watermark_info"); err != nil {
		return nil, err
	}
	for _, key := range []string{"callback_url", "external_task_id"} {
		if value, exists := options[key]; exists {
			text, ok := value.(string)
			if !ok || text != "" {
				return nil, fmt.Errorf("unsupported parameter: options.%s", key)
			}
		}
	}
	if _, exists := options["watermark_info"]; exists {
		watermark, err := klingNativeObject(options, "watermark_info")
		if err != nil {
			return nil, err
		}
		if err := klingNativeKeys(watermark, "options.watermark_info.", "enabled"); err != nil {
			return nil, err
		}
		if value, exists := watermark["enabled"]; exists {
			if enabled, ok := value.(bool); !ok || enabled {
				return nil, fmt.Errorf("watermarked outputs are not supported; options.watermark_info.enabled must be false")
			}
		}
	}
	content := make([]map[string]any, 0)
	prompts := make([]string, 0)
	if textOnly {
		prompt, ok := request["prompt"].(string)
		if !ok || strings.TrimSpace(prompt) == "" {
			return nil, fmt.Errorf("prompt is required")
		}
		prompts = append(prompts, prompt)
	}
	var contents []any
	if value, exists := request["contents"]; exists {
		var ok bool
		contents, ok = value.([]any)
		if !ok {
			return nil, fmt.Errorf("contents must be an array")
		}
	}
	if textOnly && len(contents) != 0 {
		return nil, fmt.Errorf("text-to-video reference inputs are not supported")
	}
	counts := map[string]int{}
	for _, value := range contents {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contents items must be objects")
		}
		kind, _ := item["type"].(string)
		if kind == "prompt" {
			if err := klingNativeKeys(item, "contents.", "type", "text"); err != nil {
				return nil, err
			}
			text, ok := item["text"].(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("contents prompt.text must be nonempty")
			}
			prompts = append(prompts, text)
			continue
		}
		if kind != "first_frame" && kind != "last_frame" && !(path == "/omni-video/kling-4.0" && (kind == "refer_image" || kind == "feature_video")) {
			return nil, fmt.Errorf("unsupported contents.type: %s", kind)
		}
		if err := klingNativeKeys(item, "contents.", "type", "url", "id"); err != nil {
			return nil, err
		}
		if id, exists := item["id"]; exists {
			if text, ok := id.(string); !ok || text != "" {
				return nil, fmt.Errorf("contents.id references are not supported")
			}
		}
		mediaURL, ok := item["url"].(string)
		parsed, parseErr := url.Parse(mediaURL)
		if !ok || parseErr != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return nil, fmt.Errorf("contents.url must be an HTTP(S) URL; base64 inputs are not supported")
		}
		counts[kind]++
		if (kind != "feature_video" && counts[kind] > 1) || counts["feature_video"] > 5 {
			return nil, fmt.Errorf("unsupported number of %s inputs", kind)
		}
		if kind == "feature_video" {
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": mediaURL}, "role": "reference_video"})
		} else {
			role := kind
			if role == "refer_image" {
				role = "reference_image"
			}
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": mediaURL}, "role": role})
		}
	}
	if len(prompts) == 0 {
		return nil, fmt.Errorf("contents must include a nonempty prompt in this gateway")
	}
	if imageOnly && counts["first_frame"] != 1 {
		return nil, fmt.Errorf("image-to-video requires one first_frame")
	}
	if !textOnly && len(content) == 0 {
		return nil, fmt.Errorf("contents must include supported image or video input")
	}
	model := "kling-4.0-1080p"
	if resolution == "720p" {
		model = "kling-4.0"
	}
	return map[string]any{
		"model": model, "prompt": strings.Join(prompts, "\n"), "content": content,
		"duration": duration, "seconds": strconv.Itoa(duration), "resolution": resolution,
		"aspect_ratio": ratio, "parameters": map[string]any{"duration": duration, "resolution": resolution, "aspect_ratio": ratio},
	}, nil
}
