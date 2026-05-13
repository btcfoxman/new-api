package controller

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	taskInputSnapshotStringPreviewChars = 30
	taskInputSnapshotBase64MinChars     = 256
)

var (
	taskInputSnapshotDataURIBase64Pattern = regexp.MustCompile(`(?i)data:[^,\s"']*;base64,[A-Za-z0-9+/=_-]{256,}`)
	taskInputSnapshotLongBase64Pattern    = regexp.MustCompile(`[A-Za-z0-9+/=_-]{256,}`)
)

var taskInputSnapshotFields = []string{
	"image",
	"image_url",
	"image_urls",
	"images",
	"reference_images",
	"input_reference",
	"end_image_url",
	"last_image_url",
}

func buildTaskInputSnapshot(c *gin.Context) string {
	var raw map[string]any
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return ""
	}
	return buildTaskInputSnapshotFromMap(raw)
}

func buildTaskInputSnapshotFromMap(raw map[string]any) string {
	picked := map[string]any{}
	for _, key := range taskInputSnapshotFields {
		if value, ok := raw[key]; ok && hasNonEmptyTaskInputSnapshotValue(value) {
			picked[key] = value
		}
	}
	if len(picked) == 0 {
		return ""
	}

	data, err := common.Marshal(sanitizeTaskInputSnapshotValue(picked))
	if err != nil {
		return ""
	}
	return string(data)
}

func sanitizeTaskInputSnapshotValue(value any) any {
	switch typed := value.(type) {
	case string:
		return truncateTaskInputSnapshotString(typed)
	case []any:
		sanitized := make([]any, len(typed))
		for i, item := range typed {
			sanitized[i] = sanitizeTaskInputSnapshotValue(item)
		}
		return sanitized
	case []string:
		sanitized := make([]string, len(typed))
		for i, item := range typed {
			sanitized[i] = truncateTaskInputSnapshotString(item)
		}
		return sanitized
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, item := range typed {
			sanitized[key] = sanitizeTaskInputSnapshotValue(item)
		}
		return sanitized
	default:
		return value
	}
}

func truncateTaskInputSnapshotString(s string) string {
	if len(s) < taskInputSnapshotBase64MinChars {
		return s
	}

	s = taskInputSnapshotDataURIBase64Pattern.ReplaceAllStringFunc(s, func(token string) string {
		comma := strings.IndexByte(token, ',')
		if comma < 0 || comma+1 >= len(token) {
			return truncateTaskInputSnapshotPayload(token)
		}
		return token[:comma+1] + truncateTaskInputSnapshotPayload(token[comma+1:])
	})
	return taskInputSnapshotLongBase64Pattern.ReplaceAllStringFunc(s, func(token string) string {
		if len(token) < taskInputSnapshotBase64MinChars {
			return token
		}
		return truncateTaskInputSnapshotPayload(token)
	})
}

func truncateTaskInputSnapshotPayload(s string) string {
	if len(s) <= taskInputSnapshotStringPreviewChars {
		return s
	}
	omitted := len(s) - taskInputSnapshotStringPreviewChars
	return s[:taskInputSnapshotStringPreviewChars] + fmt.Sprintf("...[truncated %d chars]", omitted)
}

func hasNonEmptyTaskInputSnapshotValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if hasNonEmptyTaskInputSnapshotValue(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if hasNonEmptyTaskInputSnapshotValue(item) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, item := range typed {
			if hasNonEmptyTaskInputSnapshotValue(item) {
				return true
			}
		}
		return false
	default:
		return value != nil
	}
}
