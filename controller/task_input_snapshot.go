package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	taskInputSnapshotStringPreviewChars = 30
	taskInputSnapshotBase64MinChars     = 256
)

var taskInputSnapshotFields = []string{
	"image",
	"image_url",
	"image_urls",
	"images",
	"image_references",
	"reference_images",
	"reference_image",
	"reference_image_url",
	"input_reference",
	"start_image_url",
	"first_image_url",
	"first_frame_image",
	"first_frame_image_url",
	"end_image",
	"end_image_url",
	"last_image_url",
	"last_frame_image",
	"last_frame_image_url",
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
	if strings.Contains(strings.ToLower(s), ";base64,") {
		return truncateTaskInputSnapshotPayload(s)
	}
	if isLikelyRawTaskInputSnapshotBase64(s) {
		return truncateTaskInputSnapshotPayload(s)
	}
	return s
}

func isLikelyRawTaskInputSnapshotBase64(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < taskInputSnapshotBase64MinChars || strings.Contains(s, "://") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+', r == '/', r == '=', r == '_', r == '-':
		case r == '\r', r == '\n', r == '\t', r == ' ':
		default:
			return false
		}
	}
	return true
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
