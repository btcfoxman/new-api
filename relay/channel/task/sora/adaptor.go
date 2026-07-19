package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
	Output    *dashScopeVideoOutput `json:"output,omitempty"`
	RequestID string                `json:"request_id,omitempty"`
}

type dashScopeVideoOutput struct {
	TaskID     string `json:"task_id"`
	TaskStatus string `json:"task_status"`
	VideoURL   string `json:"video_url,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

var (
	inlineVideoDurationPattern = regexp.MustCompile(`(?i)(^|\s)--(?:dur|duration)(?:=|\s+)(\S+)`)
	videoModeTokenReplacer     = strings.NewReplacer("-", "_", " ", "_")
)

var referenceVideoFrameFields = []string{
	"first_frame",
	"last_frame",
	"end_frame",
	"start_frame",
	"first_frame_url",
	"last_frame_url",
	"end_frame_url",
	"start_frame_url",
	"tail_frame_url",
	"first_image",
	"last_image",
	"end_image",
	"end_images",
	"image_tail",
	"first_image_url",
	"last_image_url",
	"end_image_url",
	"start_image_url",
	"tail_image_url",
	"first_frame_image",
	"last_frame_image",
	"end_frame_image",
	"first_frame_image_url",
	"last_frame_image_url",
	"end_frame_image_url",
}

var referenceVideoImageFields = []string{
	"image",
	"image_url",
	"images",
	"image_urls",
	"reference_image",
	"reference_image_url",
	"reference_image_urls",
	"reference_images",
	"input_reference",
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	if isDashScopeHappyHorsePath(c.Request.URL.Path) {
		return validateDashScopeHappyHorseRequest(c, info)
	}
	if taskErr := relaycommon.ValidateMultipartDirect(c, info); taskErr != nil {
		return taskErr
	}
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err == nil && isReferenceVideoMode(taskReq.Mode) {
		info.Action = constant.TaskActionReferenceGenerate
	}
	return nil
}

func isDashScopeHappyHorsePath(path string) bool {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(path)), "/") == "/api/v1/services/aigc/video-generation/video-synthesis"
}

func validateDashScopeHappyHorseRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var rawReq map[string]interface{}
	if err := common.UnmarshalBodyReusable(c, &rawReq); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
	}

	modelName, _ := rawReq["model"].(string)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}

	input, _ := rawReq["input"].(map[string]interface{})
	prompt, _ := input["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt, _ = rawReq["prompt"].(string)
		prompt = strings.TrimSpace(prompt)
	}
	if prompt == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	mode, _ := rawReq["mode"].(string)
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = happyHorseModeFromModel(modelName)
	}
	parameters, _ := rawReq["parameters"].(map[string]interface{})

	req := relaycommon.TaskSubmitReq{
		Model:      modelName,
		Mode:       mode,
		Prompt:     prompt,
		Parameters: parameters,
		Metadata:   rawReq,
	}
	if duration, ok := parsePositiveDuration(parameters["duration"]); ok {
		req.Duration = duration
	}
	if resolution, ok := parameters["resolution"].(string); ok {
		req.Size = strings.TrimSpace(resolution)
	}

	switch mode {
	case "r2v":
		info.Action = constant.TaskActionReferenceGenerate
	case "i2v", "video_edit":
		info.Action = constant.TaskActionGenerate
	default:
		info.Action = constant.TaskActionTextGenerate
	}
	c.Set("task_request", req)
	return nil
}

func happyHorseModeFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, mode := range []string{"video_edit", "r2v", "i2v", "t2v"} {
		if strings.HasSuffix(model, "-"+strings.ReplaceAll(mode, "_", "-")) {
			return mode
		}
	}
	return ""
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info != nil && info.TaskRelayInfo != nil && info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds := estimateTaskSeconds(req)
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func estimateTaskSeconds(req relaycommon.TaskSubmitReq) int {
	if seconds, ok := parsePositiveDuration(req.Seconds); ok {
		return seconds
	}
	if req.Duration > 0 {
		return req.Duration
	}
	if req.Parameters != nil {
		if seconds, ok := parsePositiveDuration(req.Parameters["duration"]); ok {
			return seconds
		}
	}
	if seconds, ok := parseInlineTaskDuration(req); ok {
		return seconds
	}
	return 0
}

func parseInlineTaskDuration(req relaycommon.TaskSubmitReq) (int, bool) {
	for _, text := range taskDurationTexts(req) {
		if seconds, ok := parseInlineDurationText(text); ok {
			return seconds, true
		}
	}
	return 0, false
}

func taskDurationTexts(req relaycommon.TaskSubmitReq) []string {
	texts := make([]string, 0, 1+len(req.Content))
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		texts = append(texts, prompt)
	}
	texts = append(texts, taskContentTexts(req.Content)...)
	if req.Metadata != nil {
		if text, ok := req.Metadata["text"].(string); ok && strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
		texts = append(texts, taskContentTexts(req.Metadata["content"])...)
	}
	return texts
}

func taskContentTexts(content any) []string {
	var texts []string
	appendText := func(item any) {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return
		}
		itemType, _ := itemMap["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(itemType), "text") {
			return
		}
		text, _ := itemMap["text"].(string)
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}

	switch typed := content.(type) {
	case []map[string]interface{}:
		for _, item := range typed {
			appendText(item)
		}
	case []interface{}:
		for _, item := range typed {
			appendText(item)
		}
	}
	return texts
}

func parseInlineDurationText(text string) (int, bool) {
	matches := inlineVideoDurationPattern.FindStringSubmatch(text)
	if len(matches) < 3 {
		return 0, false
	}
	return parsePositiveDuration(matches[2])
}

func parsePositiveDuration(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case int32:
		return int(typed), typed > 0
	case float64:
		return int(typed), typed > 0
	case float32:
		return int(typed), typed > 0
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return int(parsed), true
		}
		parsed, err := typed.Float64()
		return int(parsed), err == nil && parsed > 0
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		if parsed, err := strconv.Atoi(text); err == nil && parsed > 0 {
			return parsed, true
		}
		parsed, err := strconv.ParseFloat(text, 64)
		return int(parsed), err == nil && parsed > 0
	default:
		return 0, false
	}
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := strings.ToLower(c.GetHeader("Content-Type"))

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			originalModel, _ := bodyMap["model"].(string)
			bodyMap["model"] = info.UpstreamModelName
			if shouldNormalizeReferenceVideoRequest(info) {
				normalizeReferenceVideoJSONBody(bodyMap)
			}
			if isDashScopeHappyHorsePath(c.Request.URL.Path) {
				input, _ := bodyMap["input"].(map[string]interface{})
				prompt, _ := input["prompt"].(string)
				if strings.TrimSpace(prompt) == "" {
					prompt, _ = bodyMap["prompt"].(string)
					if strings.TrimSpace(prompt) != "" {
						if input == nil {
							input = map[string]interface{}{}
							bodyMap["input"] = input
						}
						input["prompt"] = prompt
					}
				} else {
					bodyMap["prompt"] = prompt
				}
				if mode, _ := bodyMap["mode"].(string); strings.TrimSpace(mode) == "" {
					inferredMode := happyHorseModeFromModel(originalModel)
					if inferredMode == "" {
						inferredMode = happyHorseModeFromModel(info.UpstreamModelName)
					}
					if inferredMode != "" {
						bodyMap["mode"] = inferredMode
					}
				}
			}
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		if shouldNormalizeReferenceVideoRequest(info) {
			normalizeReferenceVideoFormValues(url.Values(formData.Value))
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	if strings.Contains(contentType, gin.MIMEPOSTForm) {
		values, err := url.ParseQuery(string(cachedBody))
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		if shouldNormalizeReferenceVideoRequest(info) {
			normalizeReferenceVideoFormValues(values)
		}
		values.Set("model", info.UpstreamModelName)
		return strings.NewReader(values.Encode()), nil
	}

	return common.ReaderOnly(storage), nil
}

func shouldNormalizeReferenceVideoRequest(info *relaycommon.RelayInfo) bool {
	return info == nil || info.TaskRelayInfo == nil || info.Action != constant.TaskActionRemix
}

func normalizeReferenceVideoJSONBody(body map[string]interface{}) {
	if body == nil {
		return
	}
	mode, _ := body["mode"].(string)
	if !isReferenceVideoMode(mode) {
		return
	}

	seenURLs := make(map[string]struct{})
	for _, field := range referenceVideoImageFields {
		collectReferenceVideoURLs(body[field], seenURLs, nil)
	}
	collectReferenceVideoContentImageURLs(body["content"], seenURLs)

	promotedURLs := make([]string, 0, len(referenceVideoFrameFields))
	for _, field := range referenceVideoFrameFields {
		collectReferenceVideoURLs(body[field], nil, &promotedURLs)
		delete(body, field)
	}
	normalizeReferenceVideoContentRoles(body["content"])

	for _, mediaURL := range promotedURLs {
		if _, exists := seenURLs[mediaURL]; exists {
			continue
		}
		body["reference_images"] = appendReferenceVideoImage(body["reference_images"], mediaURL)
		seenURLs[mediaURL] = struct{}{}
	}
}

func normalizeReferenceVideoFormValues(values url.Values) {
	if values == nil || !isReferenceVideoMode(values.Get("mode")) {
		return
	}

	seenURLs := make(map[string]struct{})
	for _, field := range referenceVideoImageFields {
		for _, value := range values[field] {
			if mediaURL := strings.TrimSpace(value); mediaURL != "" {
				seenURLs[mediaURL] = struct{}{}
			}
		}
	}
	for _, field := range referenceVideoFrameFields {
		for _, value := range values[field] {
			mediaURL := strings.TrimSpace(value)
			if mediaURL == "" {
				continue
			}
			if _, exists := seenURLs[mediaURL]; !exists {
				values.Add("reference_images", mediaURL)
				seenURLs[mediaURL] = struct{}{}
			}
		}
		values.Del(field)
	}
}

func collectReferenceVideoURLs(value interface{}, seen map[string]struct{}, collected *[]string) {
	appendURL := func(raw string) {
		mediaURL := strings.TrimSpace(raw)
		if mediaURL == "" {
			return
		}
		if seen != nil {
			seen[mediaURL] = struct{}{}
		}
		if collected != nil {
			*collected = append(*collected, mediaURL)
		}
	}

	switch typed := value.(type) {
	case string:
		appendURL(typed)
	case []string:
		for _, item := range typed {
			appendURL(item)
		}
	case []interface{}:
		for _, item := range typed {
			collectReferenceVideoURLs(item, seen, collected)
		}
	case map[string]interface{}:
		for _, key := range []string{"url", "image_url", "image", "b64_json"} {
			if nested, exists := typed[key]; exists {
				collectReferenceVideoURLs(nested, seen, collected)
			}
		}
	}
}

func collectReferenceVideoContentImageURLs(content interface{}, seen map[string]struct{}) {
	visit := func(item interface{}) {
		itemMap, ok := item.(map[string]interface{})
		if !ok || !isReferenceVideoImageContentType(itemMap["type"]) {
			return
		}
		collectReferenceVideoURLs(itemMap["image_url"], seen, nil)
		collectReferenceVideoURLs(itemMap["url"], seen, nil)
	}

	switch typed := content.(type) {
	case []interface{}:
		for _, item := range typed {
			visit(item)
		}
	case []map[string]interface{}:
		for _, item := range typed {
			visit(item)
		}
	}
}

func normalizeReferenceVideoContentRoles(content interface{}) {
	visit := func(item interface{}) {
		itemMap, ok := item.(map[string]interface{})
		if !ok || !isReferenceVideoImageContentType(itemMap["type"]) {
			return
		}
		role, _ := itemMap["role"].(string)
		switch normalizeVideoModeToken(role) {
		case "first", "last", "start", "end", "first_frame", "last_frame", "start_frame", "end_frame", "tail_frame", "first_image", "last_image", "start_image", "end_image", "tail_image", "image_tail":
			itemMap["role"] = "reference_image"
		}
	}

	switch typed := content.(type) {
	case []interface{}:
		for _, item := range typed {
			visit(item)
		}
	case []map[string]interface{}:
		for _, item := range typed {
			visit(item)
		}
	}
}

func isReferenceVideoImageContentType(value interface{}) bool {
	itemType, _ := value.(string)
	switch normalizeVideoModeToken(itemType) {
	case "image", "image_url", "input_image":
		return true
	default:
		return false
	}
}

func normalizeVideoModeToken(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = videoModeTokenReplacer.Replace(normalized)
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}
	return normalized
}

func isReferenceVideoMode(mode string) bool {
	switch normalizeVideoModeToken(mode) {
	case "r2v", "reference", "reference2video", "reference_to_video", "reference_video", "reference_images", "reference_material", "reference_materials", "multi_reference":
		return true
	default:
		return false
	}
}

func appendReferenceVideoImage(existing interface{}, mediaURL string) []interface{} {
	values := make([]interface{}, 0)
	switch typed := existing.(type) {
	case nil:
	case []interface{}:
		values = append(values, typed...)
	case []string:
		for _, value := range typed {
			values = append(values, value)
		}
	default:
		values = append(values, typed)
	}
	return append(values, mediaURL)
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" && dResp.Output != nil {
		upstreamID = dResp.Output.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	if isDashScopeHappyHorsePath(c.Request.URL.Path) {
		requestID := strings.TrimSpace(dResp.RequestID)
		if requestID == "" {
			requestID = info.PublicTaskID
		}
		statusSource := strings.TrimSpace(dResp.Status)
		if statusSource == "" && dResp.Output != nil {
			statusSource = dResp.Output.TaskStatus
		}
		taskStatus := dashScopeTaskStatus(statusSource)
		dResp.Status = openAIVideoStatusFromDashScope(taskStatus)
		dResp.Output = &dashScopeVideoOutput{
			TaskID:     info.PublicTaskID,
			TaskStatus: taskStatus,
		}
		dResp.RequestID = requestID
		c.JSON(http.StatusOK, dResp)
		return upstreamID, responseBody, nil
	}
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

func dashScopeTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "submitted":
		return "PENDING"
	case "in_progress", "processing", "running":
		return "RUNNING"
	case "completed", "succeeded", "success", "done", "finished":
		return "SUCCEEDED"
	case "failed", "fail", "error", "cancelled", "canceled":
		return "FAILED"
	default:
		return strings.ToUpper(strings.TrimSpace(status))
	}
}

func openAIVideoStatusFromDashScope(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PENDING":
		return "queued"
	case "RUNNING":
		return "in_progress"
	case "SUCCEEDED":
		return "completed"
	case "FAILED":
		return "failed"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		// Url intentionally left empty — the caller constructs the proxy URL using the public task ID
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	return data, nil
}
