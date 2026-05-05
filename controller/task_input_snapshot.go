package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
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

	data, err := common.Marshal(picked)
	if err != nil {
		return ""
	}
	return string(data)
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
