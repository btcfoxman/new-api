package vidu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Model             string           `json:"model"`
	Images            []string         `json:"images,omitempty"`
	Videos            []string         `json:"videos,omitempty"`
	Subjects          []map[string]any `json:"subjects,omitempty"`
	AutoSubjects      bool             `json:"auto_subjects,omitempty"`
	Prompt            string           `json:"prompt,omitempty"`
	Style             string           `json:"style,omitempty"`
	Duration          int              `json:"duration,omitempty"`
	Seed              int              `json:"seed,omitempty"`
	AspectRatio       string           `json:"aspect_ratio,omitempty"`
	Resolution        string           `json:"resolution,omitempty"`
	MovementAmplitude string           `json:"movement_amplitude,omitempty"`
	Bgm               bool             `json:"bgm,omitempty"`
	Audio             bool             `json:"audio,omitempty"`
	AudioType         string           `json:"audio_type,omitempty"`
	VoiceID           string           `json:"voice_id,omitempty"`
	IsRec             bool             `json:"is_rec,omitempty"`
	Payload           string           `json:"payload,omitempty"`
	OffPeak           bool             `json:"off_peak,omitempty"`
	Watermark         bool             `json:"watermark,omitempty"`
	WmPosition        int              `json:"wm_position,omitempty"`
	WmURL             string           `json:"wm_url,omitempty"`
	MetaData          string           `json:"meta_data,omitempty"`
	CallbackUrl       string           `json:"callback_url,omitempty"`
}

type responsePayload struct {
	TaskId            string   `json:"task_id"`
	State             string   `json:"state"`
	Model             string   `json:"model"`
	Images            []string `json:"images"`
	Prompt            string   `json:"prompt"`
	Duration          int      `json:"duration"`
	Seed              int      `json:"seed"`
	Resolution        string   `json:"resolution"`
	Bgm               bool     `json:"bgm"`
	MovementAmplitude string   `json:"movement_amplitude"`
	Payload           string   `json:"payload"`
	CreatedAt         string   `json:"created_at"`
}

type taskResultResponse struct {
	State     string     `json:"state"`
	ErrCode   string     `json:"err_code"`
	Credits   int        `json:"credits"`
	Payload   string     `json:"payload"`
	Creations []creation `json:"creations"`
}

type creation struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	CoverURL string `json:"cover_url"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	rawReq := map[string]any{}
	_ = common.UnmarshalBodyReusable(c, &rawReq)
	req.Images = mergeViduImages(req.Images, collectViduImageInputs(&req, rawReq)...)

	action := resolveViduActionFromPath(c.Request.URL.Path)
	if action == "" {
		action = resolveViduActionFromMode(firstViduString(req.Mode, rawReq["mode"], mapValue(req.Metadata, "mode"), mapValue(req.Metadata, "action")))
	}
	if action == "" {
		action = constant.TaskActionTextGenerate
		if req.HasImage() {
			action = constant.TaskActionGenerate
			if info.ChannelType == constant.ChannelTypeVidu {
				if len(req.Images) == 2 {
					action = constant.TaskActionFirstTailGenerate
				} else if len(req.Images) > 2 {
					action = constant.TaskActionReferenceGenerate
				}
			}
		}
	}
	info.Action = action
	if billingModelName := normalizeViduBillingPriceModelName(taskcommon.DefaultString(info.OriginModelName, req.Model)); billingModelName != "" && billingModelName != strings.TrimSpace(info.OriginModelName) {
		c.Set("task_billing_model", billingModelName)
	}
	c.Set("task_request", req)
	return nil
}

func resolveViduActionFromPath(path string) string {
	path = strings.TrimRight(strings.ToLower(strings.TrimSpace(path)), "/")
	switch {
	case strings.HasSuffix(path, "/text2video"):
		return constant.TaskActionTextGenerate
	case strings.HasSuffix(path, "/img2video"):
		return constant.TaskActionGenerate
	case strings.HasSuffix(path, "/start-end2video"):
		return constant.TaskActionFirstTailGenerate
	case strings.HasSuffix(path, "/reference2video"):
		return constant.TaskActionReferenceGenerate
	default:
		return ""
	}
}

func resolveViduActionFromMode(mode string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(mode)), "-", "_")
	switch normalized {
	case "t2v", "text", "text2video", "text_to_video":
		return constant.TaskActionTextGenerate
	case "i2v", "image", "img2video", "image2video", "image_to_video", "first_frame":
		return constant.TaskActionGenerate
	case "i2v_first_last", "start_end", "start_end2video", "start_end_to_video", "first_last", "first_tail", "first_last_frame":
		return constant.TaskActionFirstTailGenerate
	case "r2v", "reference", "reference2video", "reference_to_video", "reference_images", "reference_material":
		return constant.TaskActionReferenceGenerate
	default:
		return ""
	}
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)
	rawReq := map[string]any{}
	_ = common.UnmarshalBodyReusable(c, &rawReq)

	body, err := a.convertToRequestPayload(&req, info, rawReq)
	if err != nil {
		return nil, err
	}

	if info.Action == constant.TaskActionReferenceGenerate {
		if strings.Contains(body.Model, "viduq2") {
			// 参考图生视频只能用 viduq2 模型, 不能带有pro或turbo后缀 https://platform.vidu.cn/docs/reference-to-video
			body.Model = "viduq2"
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var path string
	switch info.Action {
	case constant.TaskActionGenerate:
		path = "/img2video"
	case constant.TaskActionFirstTailGenerate:
		path = "/start-end2video"
	case constant.TaskActionReferenceGenerate:
		path = "/reference2video"
	default:
		path = "/text2video"
	}
	return fmt.Sprintf("%s/ent/v2%s", a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	rawReq := map[string]any{}
	_ = common.UnmarshalBodyReusable(c, &rawReq)
	payload, err := a.convertToRequestPayload(&req, info, rawReq)
	if err != nil || payload == nil {
		return nil
	}

	duration := payload.Duration
	if duration <= 0 {
		duration = 5
	}
	ratios := map[string]float64{
		"seconds": float64(duration),
	}
	if isViduQ3TurboBillingModel(info.OriginModelName) {
		c.Set("force_apply_task_other_ratios", true)
	}

	ruleModelName, ruleGroupName := viduBillingRuleTarget(info)
	rule, ok := ratio_setting.GetTaskGroupPricingRule(ruleModelName, ruleGroupName)
	if !ok || rule.BasePrice == nil || *rule.BasePrice <= 0 {
		return ratios
	}
	resolutionPrices := viduResolutionPrices(rule.Dimensions)
	if len(resolutionPrices) == 0 {
		return ratios
	}
	resolution := normalizeViduResolution(payload.Resolution, req.Size)
	if resolution == "" {
		resolution = "1080p"
	}
	price, ok := resolutionPrices[resolution]
	if !ok || price <= 0 {
		return ratios
	}
	ratios["resolution-"+resolution] = price / *rule.BasePrice
	return ratios
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var vResp responsePayload
	err = common.Unmarshal(responseBody, &vResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	if vResp.State == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task failed"), "task_failed", http.StatusBadRequest)
		return
	}

	if strings.HasPrefix(c.Request.URL.Path, "/ent/v2/") {
		publicResp := vResp
		publicResp.TaskId = info.PublicTaskID
		c.JSON(http.StatusOK, publicResp)
		return vResp.TaskId, responseBody, nil
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return vResp.TaskId, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"viduq3-turbo", "viduq2", "viduq1", "vidu2.0", "vidu1.5"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "vidu"
}

func viduBillingRuleTarget(info *relaycommon.RelayInfo) (string, string) {
	if info == nil {
		return "", ""
	}
	modelName := normalizeViduBillingPriceModelName(info.OriginModelName)
	groupName := info.UsingGroup
	lowerModelName := strings.ToLower(modelName)
	if strings.HasPrefix(lowerModelName, "viduq3-turbo") {
		if strings.Contains(lowerModelName, "official") {
			groupName = "official"
		} else if strings.Contains(lowerModelName, "stable") {
			groupName = "stable"
		}
		return "viduq3-turbo", groupName
	}
	return strings.TrimSpace(modelName), groupName
}

func normalizeViduBillingPriceModelName(modelName string) string {
	trimmedModelName := strings.TrimSpace(modelName)
	lowerModelName := strings.ToLower(trimmedModelName)
	if !strings.HasPrefix(lowerModelName, "viduq3-turbo") {
		return trimmedModelName
	}
	if strings.Contains(lowerModelName, "official") {
		return "viduq3-turbo-official"
	}
	if strings.Contains(lowerModelName, "stable") {
		return "viduq3-turbo-stable"
	}
	return "viduq3-turbo"
}

func isViduQ3TurboBillingModel(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(normalizeViduBillingPriceModelName(modelName)), "viduq3-turbo")
}

func viduResolutionPrices(dimensions map[string]any) map[string]float64 {
	raw, ok := dimensions["resolution_prices"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	prices := make(map[string]float64, len(raw))
	for key, value := range raw {
		price, ok := viduFloat(value)
		if !ok || price <= 0 {
			continue
		}
		prices[normalizeViduResolution(key, "")] = price
	}
	return prices
}

func viduFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func normalizeViduResolution(values ...string) string {
	for _, value := range values {
		text := strings.ToLower(strings.TrimSpace(value))
		if text == "" {
			continue
		}
		text = strings.ReplaceAll(text, "*", "x")
		text = strings.ReplaceAll(text, " ", "")
		switch text {
		case "540p", "540":
			return "540p"
		case "720p", "720":
			return "720p"
		case "1080p", "1080":
			return "1080p"
		case "960x540", "540x960":
			return "540p"
		case "1280x720", "720x1280":
			return "720p"
		case "1920x1080", "1080x1920":
			return "1080p"
		}
	}
	return ""
}

// ============================
// helpers
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo, rawReq map[string]any) (*requestPayload, error) {
	duration := taskcommon.DefaultInt(req.Duration, 5)
	if req.Seconds != "" {
		if parsed, err := strconv.Atoi(req.Seconds); err == nil && parsed > 0 {
			duration = parsed
		}
	}
	r := requestPayload{
		Model:             taskcommon.DefaultString(info.UpstreamModelName, "viduq1"),
		Images:            mergeViduImages(req.Images, collectViduImageInputs(req, rawReq)...),
		Prompt:            req.Prompt,
		Duration:          duration,
		Resolution:        taskcommon.DefaultString(req.Size, "1080p"),
		MovementAmplitude: "auto",
		Bgm:               false,
	}
	if len(rawReq) > 0 {
		rawBytes, err := common.Marshal(rawReq)
		if err != nil {
			return nil, errors.Wrap(err, "marshal raw request failed")
		}
		if err := common.Unmarshal(rawBytes, &r); err != nil {
			return nil, errors.Wrap(err, "unmarshal raw request failed")
		}
		r.Model = taskcommon.DefaultString(info.UpstreamModelName, r.Model)
		if r.Model == "" {
			r.Model = "viduq1"
		}
		if r.Prompt == "" {
			r.Prompt = req.Prompt
		}
		if r.Duration == 0 {
			r.Duration = duration
		}
		if r.Resolution == "" {
			r.Resolution = taskcommon.DefaultString(req.Size, "1080p")
		}
		if r.MovementAmplitude == "" {
			r.MovementAmplitude = "auto"
		}
		r.Images = mergeViduImages(r.Images, collectViduImageInputs(req, rawReq)...)
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	r.Model = taskcommon.DefaultString(info.UpstreamModelName, r.Model)
	if info.Action == constant.TaskActionTextGenerate {
		r.Images = nil
	}
	return &r, nil
}

func mapValue(values map[string]interface{}, key string) any {
	if values == nil {
		return nil
	}
	return values[key]
}

func firstViduString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func collectViduImageInputs(req *relaycommon.TaskSubmitReq, rawReq map[string]any) []string {
	var images []string
	if req != nil {
		images = append(images, req.Images...)
		images = append(images, extractViduURLValues(req.Image)...)
		images = append(images, extractViduURLValues(req.InputReference)...)
		images = append(images, extractViduContentImages(req.Content)...)
		if req.Metadata != nil {
			for _, key := range []string{"images", "image_urls", "image_url", "image", "input_reference", "reference_images"} {
				images = append(images, extractViduURLValues(req.Metadata[key])...)
			}
			images = append(images, extractViduContentImages(req.Metadata["content"])...)
			images = append(images, extractViduURLValues(req.Metadata["end_image_url"])...)
			images = append(images, extractViduURLValues(req.Metadata["last_image_url"])...)
		}
	}
	if rawReq != nil {
		for _, key := range []string{"images", "image_urls", "image_url", "image", "input_reference", "reference_images"} {
			images = append(images, extractViduURLValues(rawReq[key])...)
		}
		images = append(images, extractViduContentImages(rawReq["content"])...)
		images = append(images, extractViduURLValues(rawReq["end_image_url"])...)
		images = append(images, extractViduURLValues(rawReq["last_image_url"])...)
	}
	return mergeViduImages(nil, images...)
}

func extractViduContentImages(content any) []string {
	var images []string
	switch typed := content.(type) {
	case []map[string]interface{}:
		for _, item := range typed {
			images = append(images, extractViduImageFromContentItem(item)...)
		}
	case []any:
		for _, item := range typed {
			images = append(images, extractViduImageFromContentItem(item)...)
		}
	case map[string]any:
		images = append(images, extractViduImageFromContentItem(typed)...)
	}
	return images
}

func extractViduImageFromContentItem(item any) []string {
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}
	itemType := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", itemMap["type"])))
	if itemType != "" && itemType != "image_url" && itemType != "input_image" && itemType != "image" {
		return nil
	}
	var images []string
	for _, key := range []string{"image_url", "url", "image"} {
		images = append(images, extractViduURLValues(itemMap[key])...)
	}
	return images
}

func extractViduURLValues(value any) []string {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return []string{text}
	case []string:
		var out []string
		for _, item := range typed {
			out = append(out, extractViduURLValues(item)...)
		}
		return out
	case []any:
		var out []string
		for _, item := range typed {
			out = append(out, extractViduURLValues(item)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, key := range []string{"url", "image_url", "image"} {
			out = append(out, extractViduURLValues(typed[key])...)
		}
		return out
	default:
		return nil
	}
}

func mergeViduImages(existing []string, values ...string) []string {
	out := make([]string, 0, len(existing)+len(values))
	seen := map[string]bool{}
	for _, item := range append(existing, values...) {
		text := strings.TrimSpace(item)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	var taskResp taskResultResponse
	err := common.Unmarshal(respBody, &taskResp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	state := taskResp.State
	switch state {
	case "created", "queueing":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "success":
		taskInfo.Status = model.TaskStatusSuccess
		if len(taskResp.Creations) > 0 {
			taskInfo.Url = taskResp.Creations[0].URL
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		if taskResp.ErrCode != "" {
			taskInfo.Reason = taskResp.ErrCode
		}
	default:
		return nil, fmt.Errorf("unknown task state: %s", state)
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var viduResp taskResultResponse
	if err := common.Unmarshal(originTask.Data, &viduResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal vidu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if len(viduResp.Creations) > 0 && viduResp.Creations[0].URL != "" {
		openAIVideo.SetMetadata("url", viduResp.Creations[0].URL)
	}

	if viduResp.State == "failed" && viduResp.ErrCode != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: viduResp.ErrCode,
			Code:    viduResp.ErrCode,
		}
	}

	return common.Marshal(openAIVideo)
}
