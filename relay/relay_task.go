package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	//PerCallPrice   types.PriceData
}

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			sizeStr, _ := taskData["size"].(string)
			if info.PriceData.OtherRatios == nil {
				info.PriceData.OtherRatios = map[string]float64{}
			}
			info.PriceData.OtherRatios["seconds"] = float64(seconds)
			info.PriceData.OtherRatios["size"] = 1
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.OtherRatios["size"] = 1.666667
			}
		}
	}

	return nil
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度
	//    对“固定按次”分组，不应用 seconds/size 等倍率，避免与分组计费模式冲突。
	if !common.StringsContains(constant.TaskPricePatches, modelName) && helper.ShouldApplyTaskOtherRatios(info) {
		for _, ra := range info.PriceData.OtherRatios {
			if ra != 1.0 {
				info.PriceData.Quota = int(float64(info.PriceData.Quota) * ra)
			}
		}
	}

	// 7. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		// 基于调整后的 ratios 重新计算 quota
		finalQuota = recalcQuotaFromRatios(info, adjustedRatios)
		info.PriceData.OtherRatios = adjustedRatios
		info.PriceData.Quota = finalQuota
	}

	return &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) int {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.Quota
	// 先除掉原有的 OtherRatios 恢复基础额度
	for _, ra := range info.PriceData.OtherRatios {
		if ra != 1.0 && ra > 0 {
			baseQuota = int(float64(baseQuota) / ra)
		}
	}
	// 应用新的 ratios
	result := float64(baseQuota)
	for _, ra := range ratios {
		if ra != 1.0 {
			result *= ra
		}
	}
	return int(result)
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")
	isAliOfficialTaskAPI := strings.HasPrefix(c.Request.URL.Path, "/api/v1/tasks/")
	isViduOfficialTaskAPI := strings.HasPrefix(c.Request.URL.Path, "/ent/v2/tasks/")
	isDoubaoOfficialTaskAPI := strings.HasPrefix(c.Request.URL.Path, "/api/v3/contents/generations/tasks/")

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	if isViduOfficialTaskAPI {
		respBody = buildViduOfficialTaskResponse(originTask)
		return
	}

	if isAliOfficialTaskAPI {
		respBody = buildAliOfficialTaskResponse(originTask)
		return
	}

	if isDoubaoOfficialTaskAPI {
		respBody = buildDoubaoOfficialTaskResponse(originTask)
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

func buildViduOfficialTaskResponse(originTask *model.Task) []byte {
	if len(originTask.Data) > 0 {
		var payload map[string]any
		if err := common.Unmarshal(originTask.Data, &payload); err == nil && payload != nil {
			payload["id"] = originTask.TaskID
			if _, ok := payload["task_id"]; ok {
				payload["task_id"] = originTask.TaskID
			}
			data, err := common.Marshal(payload)
			if err == nil {
				return data
			}
		}
	}

	state := "created"
	switch originTask.Status {
	case model.TaskStatusInProgress:
		state = "processing"
	case model.TaskStatusSuccess:
		state = "success"
	case model.TaskStatusFailure:
		state = "failed"
	}
	data, _ := common.Marshal(map[string]any{
		"id":        originTask.TaskID,
		"state":     state,
		"err_code":  originTask.FailReason,
		"creations": []any{},
	})
	return data
}

func buildDoubaoOfficialTaskResponse(originTask *model.Task) []byte {
	asString := func(v any) string {
		if v == nil {
			return ""
		}
		s, ok := v.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(s)
	}

	toInt := func(v any) int {
		if v == nil {
			return 0
		}
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		case string:
			if n == "" {
				return 0
			}
			i64, err := strconv.Atoi(n)
			if err == nil {
				return i64
			}
			f64, err := strconv.ParseFloat(n, 64)
			if err == nil {
				return int(f64)
			}
		case json.Number:
			i64, err := n.Int64()
			if err == nil {
				return int(i64)
			}
		}
		return 0
	}

	extractVideoURL := func(payload map[string]any, fallback string) string {
		if payload == nil {
			return fallback
		}
		if v, ok := payload["content"]; ok {
			if content, ok := v.(map[string]any); ok {
				if videoURL := asString(content["video_url"]); videoURL != "" {
					return videoURL
				}
			}
		}
		if v := asString(payload["video_url"]); v != "" {
			return v
		}
		return strings.TrimSpace(fallback)
	}

	asMap := func(v any) map[string]any {
		m, _ := v.(map[string]any)
		return m
	}

	firstString := func(values ...any) string {
		for _, value := range values {
			if s := asString(value); s != "" {
				return s
			}
		}
		return ""
	}

	firstInt := func(values ...any) int {
		for _, value := range values {
			if value == nil {
				continue
			}
			if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
				continue
			}
			return toInt(value)
		}
		return 0
	}

	firstPositiveInt := func(values ...any) int {
		for _, value := range values {
			if n := toInt(value); n > 0 {
				return n
			}
		}
		return 0
	}

	toBool := func(v any) (bool, bool) {
		if v == nil {
			return false, false
		}
		switch b := v.(type) {
		case bool:
			return b, true
		case string:
			s := strings.TrimSpace(strings.ToLower(b))
			switch s {
			case "true", "1", "yes", "y", "on":
				return true, true
			case "false", "0", "no", "n", "off":
				return false, true
			}
		case int:
			return b != 0, true
		case int64:
			return b != 0, true
		case float64:
			return b != 0, true
		case json.Number:
			n, err := b.Int64()
			if err == nil {
				return n != 0, true
			}
		}
		return false, false
	}

	firstBool := func(defaultValue bool, values ...any) bool {
		for _, value := range values {
			if b, ok := toBool(value); ok {
				return b
			}
		}
		return defaultValue
	}

	copyMap := func(v any) map[string]any {
		source := asMap(v)
		if source == nil {
			return nil
		}
		copied := make(map[string]any, len(source))
		for key, value := range source {
			copied[key] = value
		}
		return copied
	}

	parseSize := func(size string) (int, int, bool) {
		size = strings.ToLower(strings.TrimSpace(size))
		parts := strings.Split(size, "x")
		if len(parts) != 2 {
			return 0, 0, false
		}
		w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || w <= 0 {
			return 0, 0, false
		}
		h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || h <= 0 {
			return 0, 0, false
		}
		return w, h, true
	}

	resolutionFromSize := func(size string) string {
		w, h, ok := parseSize(size)
		if !ok {
			return ""
		}
		shortSide := w
		if h < shortSide {
			shortSide = h
		}
		return strconv.Itoa(shortSide) + "p"
	}

	ratioFromSize := func(size string) string {
		w, h, ok := parseSize(size)
		if !ok {
			return ""
		}
		gcd := func(a, b int) int {
			for b != 0 {
				a, b = b, a%b
			}
			return a
		}
		divisor := gcd(w, h)
		return strconv.Itoa(w/divisor) + ":" + strconv.Itoa(h/divisor)
	}

	legacy := dto.TaskResponse[any]{
		Code:    dto.TaskSuccessCode,
		Message: "",
		Data:    TaskModel2Dto(originTask),
	}
	legacyResp, _ := common.Marshal(legacy)
	respPayload := map[string]any{
		"code":    dto.TaskSuccessCode,
		"message": "",
	}
	if err := common.Unmarshal(legacyResp, &respPayload); err != nil {
		respPayload["code"] = dto.TaskSuccessCode
		respPayload["message"] = ""
		respPayload["data"] = TaskModel2Dto(originTask)
	}

	var upstreamPayload map[string]any
	if len(originTask.Data) > 0 {
		_ = common.Unmarshal(originTask.Data, &upstreamPayload)
	}

	content := map[string]any{}
	videoURL := extractVideoURL(upstreamPayload, originTask.GetResultURL())
	if videoURL != "" {
		content["video_url"] = videoURL
	}

	modelName := originTask.Properties.OriginModelName
	if modelName == "" {
		if v := asString(upstreamPayload["model"]); v != "" {
			modelName = v
		}
	}

	officialStatus := mapTaskStatusToSimple(originTask.Status)
	if v := asString(upstreamPayload["status"]); v != "" {
		switch v {
		case "completed":
			officialStatus = "succeeded"
		case "succeeded", "processing", "running", "queued", "failed":
			officialStatus = v
		case "fail":
			officialStatus = "failed"
		case "in_progress":
			officialStatus = "processing"
		case "not_start", "not started":
			officialStatus = "queued"
		default:
			officialStatus = v
		}
	}

	metadata := asMap(upstreamPayload["metadata"])
	parameters := asMap(upstreamPayload["parameters"])
	size := firstString(upstreamPayload["size"], metadata["size"], parameters["size"])

	resolution := firstString(
		upstreamPayload["resolution"],
		upstreamPayload["quality"],
		metadata["resolution"],
		metadata["quality"],
		parameters["resolution"],
		parameters["quality"],
		resolutionFromSize(size),
	)
	ratio := firstString(
		upstreamPayload["ratio"],
		upstreamPayload["aspect_ratio"],
		metadata["ratio"],
		metadata["aspect_ratio"],
		parameters["ratio"],
		parameters["aspect_ratio"],
		ratioFromSize(size),
	)
	if resolution == "" {
		resolution = "720p"
	}
	if ratio == "" {
		ratio = "16:9"
	}

	duration := toInt(upstreamPayload["duration"])
	if duration == 0 {
		duration = toInt(upstreamPayload["seconds"])
	}
	if duration == 0 {
		duration = toInt(metadata["duration"])
	}
	if duration == 0 {
		duration = toInt(metadata["seconds"])
	}
	if duration == 0 {
		duration = toInt(parameters["duration"])
	}
	if duration == 0 {
		duration = toInt(parameters["seconds"])
	}
	if duration == 0 {
		usage := asMap(upstreamPayload["usage"])
		duration = toInt(usage["duration_seconds"])
	}

	usage := copyMap(upstreamPayload["usage"])
	if usage == nil {
		usage = map[string]any{}
	}
	if _, ok := usage["completion_tokens"]; !ok {
		if n := firstPositiveInt(upstreamPayload["completion_tokens"]); n > 0 {
			usage["completion_tokens"] = n
		}
	}
	if _, ok := usage["completion_tokens"]; !ok {
		usage["completion_tokens"] = 0
	}
	if _, ok := usage["total_tokens"]; !ok {
		if n := firstPositiveInt(upstreamPayload["total_tokens"]); n > 0 {
			usage["total_tokens"] = n
		}
	}
	if _, ok := usage["total_tokens"]; !ok {
		usage["total_tokens"] = 0
	}

	createdAt := firstPositiveInt(upstreamPayload["created_at"], originTask.CreatedAt, originTask.SubmitTime)
	updatedAt := firstPositiveInt(upstreamPayload["updated_at"], upstreamPayload["completed_at"], originTask.UpdatedAt, originTask.FinishTime, createdAt)
	seed := firstInt(upstreamPayload["seed"], metadata["seed"], parameters["seed"])
	framesPerSecond := firstPositiveInt(
		upstreamPayload["framespersecond"],
		upstreamPayload["frames_per_second"],
		upstreamPayload["fps"],
		metadata["framespersecond"],
		metadata["frames_per_second"],
		metadata["fps"],
		parameters["framespersecond"],
		parameters["frames_per_second"],
		parameters["fps"],
	)
	if framesPerSecond == 0 {
		framesPerSecond = 24
	}
	serviceTier := firstString(upstreamPayload["service_tier"], metadata["service_tier"], parameters["service_tier"])
	if serviceTier == "" {
		serviceTier = "default"
	}
	executionExpiresAfter := firstPositiveInt(upstreamPayload["execution_expires_after"], metadata["execution_expires_after"], parameters["execution_expires_after"])
	if executionExpiresAfter == 0 {
		executionExpiresAfter = 172800
	}
	generateAudio := firstBool(true, upstreamPayload["generate_audio"], metadata["generate_audio"], parameters["generate_audio"])
	draft := firstBool(false, upstreamPayload["draft"], metadata["draft"], parameters["draft"])
	priority := firstInt(upstreamPayload["priority"], metadata["priority"], parameters["priority"])
	tools, ok := upstreamPayload["tools"].([]any)
	if !ok {
		tools = []any{}
	}
	errorObj := copyMap(upstreamPayload["error"])
	if errorObj == nil && originTask.Status == model.TaskStatusFailure {
		errorObj = map[string]any{
			"code":    "failed",
			"message": strings.TrimSpace(originTask.FailReason),
		}
	}
	if errorObj != nil {
		if _, ok := errorObj["code"]; !ok {
			errorObj["code"] = "failed"
		}
		if _, ok := errorObj["message"]; !ok {
			errorObj["message"] = strings.TrimSpace(originTask.FailReason)
		}
	}

	respPayload["id"] = originTask.TaskID
	respPayload["model"] = modelName
	respPayload["status"] = officialStatus
	respPayload["content"] = content
	respPayload["usage"] = usage
	respPayload["created_at"] = createdAt
	respPayload["updated_at"] = updatedAt
	respPayload["seed"] = seed
	respPayload["resolution"] = resolution
	respPayload["ratio"] = ratio
	respPayload["duration"] = duration
	respPayload["framespersecond"] = framesPerSecond
	respPayload["service_tier"] = serviceTier
	respPayload["execution_expires_after"] = executionExpiresAfter
	respPayload["generate_audio"] = generateAudio
	respPayload["draft"] = draft
	respPayload["priority"] = priority
	respPayload["tools"] = tools
	respPayload["error"] = errorObj

	if out, err := common.Marshal(respPayload); err == nil {
		return out
	}
	fallbackPayload := map[string]any{
		"code":                    dto.TaskSuccessCode,
		"message":                 "",
		"data":                    nil,
		"id":                      originTask.TaskID,
		"model":                   modelName,
		"status":                  officialStatus,
		"content":                 content,
		"usage":                   usage,
		"created_at":              createdAt,
		"updated_at":              updatedAt,
		"seed":                    seed,
		"resolution":              resolution,
		"ratio":                   ratio,
		"duration":                duration,
		"framespersecond":         framesPerSecond,
		"service_tier":            serviceTier,
		"execution_expires_after": executionExpiresAfter,
		"generate_audio":          generateAudio,
		"draft":                   draft,
		"priority":                priority,
		"tools":                   tools,
		"error":                   errorObj,
	}
	out, _ := common.Marshal(fallbackPayload)
	return out
}

func buildAliOfficialTaskResponse(originTask *model.Task) []byte {
	if len(originTask.Data) > 0 {
		var payload map[string]any
		if err := common.Unmarshal(originTask.Data, &payload); err == nil && payload != nil {
			output, _ := payload["output"].(map[string]any)
			if output == nil {
				output = map[string]any{}
			}
			output["task_id"] = originTask.TaskID
			if _, ok := output["task_status"]; !ok {
				output["task_status"] = aliTaskStatus(originTask.Status)
			}
			if existingVideoURL, ok := output["video_url"].(string); !ok || strings.TrimSpace(existingVideoURL) == "" {
				if videoURL, ok := payload["video_url"].(string); ok && strings.TrimSpace(videoURL) != "" {
					output["video_url"] = videoURL
				}
			}
			payload["output"] = output
			data, err := common.Marshal(payload)
			if err == nil {
				return data
			}
		}
	}

	output := map[string]any{
		"task_id":     originTask.TaskID,
		"task_status": aliTaskStatus(originTask.Status),
		"message":     originTask.FailReason,
	}
	if originTask.Status == model.TaskStatusSuccess {
		if videoURL := strings.TrimSpace(originTask.GetResultURL()); videoURL != "" {
			output["video_url"] = videoURL
		}
	}

	data, _ := common.Marshal(map[string]any{"output": output})
	return data
}

func aliTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSubmitted, model.TaskStatusQueued:
		return "PENDING"
	case model.TaskStatusInProgress:
		return "RUNNING"
	case model.TaskStatusSuccess:
		return "SUCCEEDED"
	case model.TaskStatusFailure:
		return "FAILED"
	default:
		return "PENDING"
	}
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Prompt:     task.Prompt,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.Data,
	}
}
