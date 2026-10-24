package common

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	gatewaycommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const ViduQ4NormalizedContextKey = "vidu_q4_normalized_request"

func NormalizeViduQ4TaskRequest(c *gin.Context, info *RelayInfo) error {
	req, err := GetTaskRequest(c)
	if err != nil || !IsViduQ4Model(req.Model) {
		return err
	}
	modelName := req.Model
	if info != nil && IsViduQ4Model(info.OriginModelName) {
		modelName = info.OriginModelName
	}
	raw := map[string]any{}
	if err := gatewaycommon.UnmarshalBodyReusable(c, &raw); err != nil {
		return fmt.Errorf("invalid Vidu Q4 request")
	}
	normalized, action, err := NormalizeViduQ4Request(raw, modelName, c.Request.URL.Path)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("invalid Vidu Q4 request")
	}
	var canonical TaskSubmitReq
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return fmt.Errorf("invalid Vidu Q4 request")
	}
	canonical.FillPromptFromContent()
	c.Set("task_request", canonical)
	c.Set(ViduQ4NormalizedContextKey, normalized)
	if info != nil {
		info.Action = action
	}
	return nil
}

func IsViduQ4Model(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "vidu-q4", "vidu-q4-540p":
		return true
	}
	return false
}

// NormalizeViduQ4Request keeps the public model, requested duration and all
// unique reference URLs aligned before submission and billing. Other Vidu
// generations retain their existing contracts.
func NormalizeViduQ4Request(raw map[string]any, modelName, path string) (map[string]any, string, error) {
	if !IsViduQ4Model(modelName) {
		return raw, "", nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, "", fmt.Errorf("invalid Vidu Q4 request")
	}
	body := map[string]any{}
	if err := json.Unmarshal(encoded, &body); err != nil {
		return nil, "", fmt.Errorf("invalid Vidu Q4 request")
	}
	if text, ok := body["metadata"].(string); ok && strings.TrimSpace(text) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(text), &metadata); err != nil || metadata == nil {
			return nil, "", fmt.Errorf("Vidu Q4 metadata must contain a JSON object")
		}
		body["metadata"] = metadata
	}
	path = strings.TrimRight(strings.ToLower(path), "/")
	if path == "/ent/v2/text2video" || path == "/ent/v2/start-end2video" {
		return nil, "", fmt.Errorf("Vidu Q4 supports image and reference generation; text-only and first/last-frame generation are unavailable")
	}
	containers := []map[string]any{body}
	for _, key := range []string{"metadata", "parameters"} {
		if values, ok := body[key].(map[string]any); ok {
			containers = append(containers, values)
		}
	}
	var images, audio, video []string
	mode := ""
	duration := 0
	resolution := "720p"
	if strings.HasSuffix(strings.ToLower(modelName), "-540p") {
		resolution = "540p"
	}
	for _, values := range containers {
		if mode == "" {
			mode, _ = values["mode"].(string)
			mode = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(mode)), "-", "_")
		}
		for _, key := range []string{"end_image_url", "last_image_url", "last_frame", "tail_image", "end_frame"} {
			if len(viduQ4URLs(values[key])) > 0 {
				return nil, "", fmt.Errorf("Vidu Q4 does not support a designated last frame; use ordinary reference images")
			}
		}
		for _, key := range []string{"images", "image_urls", "image_url", "image", "input_reference", "reference_images", "first_frame", "first_image"} {
			images = append(images, viduQ4URLs(values[key])...)
		}
		for _, key := range []string{"audio_urls", "audio_url", "audios", "reference_audio"} {
			audio = append(audio, viduQ4URLs(values[key])...)
		}
		for _, key := range []string{"videos", "video_urls", "video_url", "video", "reference_videos"} {
			video = append(video, viduQ4URLs(values[key])...)
		}
		if items, ok := values["content"].([]any); ok {
			for _, item := range items {
				part, ok := item.(map[string]any)
				if !ok {
					continue
				}
				kind, _ := part["type"].(string)
				role, _ := part["role"].(string)
				if strings.Contains(strings.ToLower(role), "last_frame") || strings.Contains(strings.ToLower(role), "tail_frame") {
					return nil, "", fmt.Errorf("Vidu Q4 does not support a designated last frame")
				}
				switch strings.ToLower(kind) {
				case "image_url", "image", "input_image":
					images = append(images, viduQ4URLs(part)...)
				case "audio_url", "audio", "input_audio":
					audio = append(audio, viduQ4URLs(part)...)
				case "video_url", "video", "input_video":
					video = append(video, viduQ4URLs(part)...)
				}
			}
		}
		for _, key := range []string{"duration", "seconds"} {
			value, exists := values[key]
			if !exists || value == nil || value == "" {
				continue
			}
			parsed, err := strconv.Atoi(fmt.Sprint(value))
			if err != nil || parsed < 3 || parsed > 16 || (duration != 0 && duration != parsed) {
				return nil, "", fmt.Errorf("Vidu Q4 duration must be one consistent integer from 3 to 16 seconds")
			}
			duration = parsed
		}
		for _, key := range []string{"resolution", "size"} {
			if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
				if viduQ4Resolution(value) != resolution {
					return nil, "", fmt.Errorf("Vidu Q4 resolution must match model %s (%s)", modelName, resolution)
				}
			}
		}
	}
	images, audio, video = viduQ4Unique(images), viduQ4Unique(audio), viduQ4Unique(video)
	if len(images) < 1 || len(images) > 15 || len(audio) > 3 || len(video) > 0 {
		return nil, "", fmt.Errorf("Vidu Q4 requires 1 to 15 images, accepts at most 3 reference audio clips, and does not accept reference videos")
	}
	switch mode {
	case "", "i2v", "image_to_video", "reference_images", "reference_material", "r2v", "reference", "reference2video":
	default:
		return nil, "", fmt.Errorf("Vidu Q4 supports image/reference modes only")
	}
	if duration == 0 {
		duration = 3
	}
	action := constant.TaskActionGenerate
	if len(images) > 1 || len(audio) > 0 || strings.HasPrefix(mode, "reference") || mode == "r2v" || path == "/ent/v2/reference2video" {
		action, mode = constant.TaskActionReferenceGenerate, "reference_images"
		if len(audio) > 0 {
			mode = "reference_material"
		}
	} else {
		mode = "i2v"
	}
	body["model"], body["images"], body["audio_urls"] = modelName, images, audio
	body["duration"], body["seconds"], body["resolution"], body["mode"] = duration, strconv.Itoa(duration), resolution, mode
	// These fields have been merged and validated above. A second metadata
	// unmarshal must not overwrite canonical media, duration or public model.
	for _, values := range containers[1:] {
		for _, key := range []string{"model", "model_name", "images", "audio_urls", "duration", "seconds", "resolution", "size", "mode"} {
			delete(values, key)
		}
	}
	return body, action, nil
}

func viduQ4Resolution(value string) string {
	value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "*", "x")
	if value == "540" || value == "540p" {
		return "540p"
	}
	if value == "720" || value == "720p" {
		return "720p"
	}
	parts := strings.Split(value, "x")
	if len(parts) == 2 {
		w, e1 := strconv.Atoi(parts[0])
		h, e2 := strconv.Atoi(parts[1])
		if e1 == nil && e2 == nil && w > 0 && h > 0 {
			if w > h {
				w = h
			}
			if w == 540 {
				return "540p"
			}
			if w == 720 {
				return "720p"
			}
		}
	}
	return ""
}

func viduQ4URLs(value any) []string {
	switch value := value.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			return []string{strings.TrimSpace(value)}
		}
	case []any:
		var urls []string
		for _, item := range value {
			urls = append(urls, viduQ4URLs(item)...)
		}
		return urls
	case map[string]any:
		var urls []string
		for _, key := range []string{"url", "image_url", "audio_url", "video_url", "image", "audio", "video"} {
			urls = append(urls, viduQ4URLs(value[key])...)
		}
		return urls
	}
	return nil
}

func viduQ4Unique(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
