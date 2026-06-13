package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

const (
	responsesResponseContextKey     = "openai_responses_response"
	responsesResponseBodyContextKey = "openai_responses_response_body"
	responsesResponseIDContextKey   = "openai_responses_response_id"
	responsesTaskIDContextKey       = "openai_responses_task_id"
	maxResponsesTaskIDLength        = 180
)

func ResponsesTaskID(responseID string) string {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || len(responseID) <= maxResponsesTaskIDLength {
		return responseID
	}
	sum := sha256.Sum256([]byte(responseID))
	return "resp_task_" + hex.EncodeToString(sum[:])
}

func IsResponsesBackgroundRequest(request *dto.OpenAIResponsesRequest) bool {
	return responsesRequestBackground(request)
}

func IsResponsesAsyncTaskRequest(request *dto.OpenAIResponsesRequest) bool {
	if request == nil {
		return false
	}
	if responsesRequestBackgroundExplicitFalse(request) {
		return false
	}
	return responsesRequestBackground(request) || responsesRequestHasImageGeneration(request)
}

func IsResponsesImageGenerationRequest(request *dto.OpenAIResponsesRequest) bool {
	return responsesRequestHasImageGeneration(request)
}

func CaptureResponsesResponse(c *gin.Context, response *dto.OpenAIResponsesResponse, responseBody []byte) {
	if c == nil || response == nil {
		return
	}
	c.Set(responsesResponseContextKey, response)
	responseID := response.ID
	if requestResponseID := strings.TrimSpace(c.Param("response_id")); requestResponseID != "" {
		responseID = requestResponseID
	}
	if responseID != "" {
		c.Set(responsesResponseIDContextKey, responseID)
		c.Set(responsesTaskIDContextKey, ResponsesTaskID(responseID))
	}
	if len(responseBody) > 0 {
		c.Set(responsesResponseBodyContextKey, append([]byte(nil), responseBody...))
	}
}

func capturedResponsesResponse(c *gin.Context) (*dto.OpenAIResponsesResponse, []byte, bool) {
	if c == nil {
		return nil, nil, false
	}
	rawResponse, ok := c.Get(responsesResponseContextKey)
	if !ok {
		return nil, nil, false
	}
	response, ok := rawResponse.(*dto.OpenAIResponsesResponse)
	if !ok || response == nil {
		return nil, nil, false
	}
	var responseBody []byte
	if raw, ok := c.Get(responsesResponseBodyContextKey); ok {
		if body, ok := raw.([]byte); ok {
			responseBody = append([]byte(nil), body...)
		}
	}
	return response, responseBody, true
}

func responsesRequestBackground(request *dto.OpenAIResponsesRequest) bool {
	if request == nil || len(request.Background) == 0 {
		return false
	}
	var background bool
	if err := json.Unmarshal(request.Background, &background); err == nil {
		return background
	}
	var backgroundString string
	if err := json.Unmarshal(request.Background, &backgroundString); err == nil {
		return strings.EqualFold(strings.TrimSpace(backgroundString), "true")
	}
	return false
}

func responsesRequestBackgroundExplicitFalse(request *dto.OpenAIResponsesRequest) bool {
	if request == nil || len(request.Background) == 0 {
		return false
	}
	var background bool
	if err := json.Unmarshal(request.Background, &background); err == nil {
		return !background
	}
	var backgroundString string
	if err := json.Unmarshal(request.Background, &backgroundString); err == nil {
		return strings.EqualFold(strings.TrimSpace(backgroundString), "false")
	}
	return false
}

func responsesRequestHasImageGeneration(request *dto.OpenAIResponsesRequest) bool {
	if request == nil {
		return false
	}
	for _, tool := range request.GetToolsMap() {
		if strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["type"])), dto.ResponsesOutputTypeImageGenerationCall) ||
			strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["type"])), "image_generation") {
			return true
		}
	}
	if len(request.ToolChoice) > 0 {
		var toolChoice map[string]any
		if err := json.Unmarshal(request.ToolChoice, &toolChoice); err == nil {
			if strings.EqualFold(strings.TrimSpace(common.Interface2String(toolChoice["type"])), dto.ResponsesOutputTypeImageGenerationCall) ||
				strings.EqualFold(strings.TrimSpace(common.Interface2String(toolChoice["type"])), "image_generation") {
				return true
			}
		}
		var toolChoiceString string
		if err := json.Unmarshal(request.ToolChoice, &toolChoiceString); err == nil {
			return strings.EqualFold(strings.TrimSpace(toolChoiceString), dto.ResponsesOutputTypeImageGenerationCall) ||
				strings.EqualFold(strings.TrimSpace(toolChoiceString), "image_generation")
		}
	}
	return false
}

func responsesStatusText(response *dto.OpenAIResponsesResponse) string {
	if response == nil || len(response.Status) == 0 {
		return ""
	}
	var status string
	if err := json.Unmarshal(response.Status, &status); err == nil {
		return strings.ToLower(strings.TrimSpace(status))
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(string(response.Status)), `"`))
}

func responsesFailureReason(response *dto.OpenAIResponsesResponse) string {
	if response == nil {
		return ""
	}
	if openaiError := response.GetOpenAIError(); openaiError != nil {
		if openaiError.Message != "" {
			return openaiError.Message
		}
		if openaiError.Code != nil {
			return fmt.Sprintf("%v", openaiError.Code)
		}
		if openaiError.Type != "" {
			return openaiError.Type
		}
	}
	return ""
}

func responsesTaskStatus(response *dto.OpenAIResponsesResponse) model.TaskStatus {
	status := responsesStatusText(response)
	switch status {
	case "queued", "pending", "submitted":
		return model.TaskStatusQueued
	case "in_progress", "running", "processing", "incomplete":
		return model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		return model.TaskStatusSuccess
	case "failed", "failure", "cancelled", "canceled", "expired":
		return model.TaskStatusFailure
	default:
		if responsesFailureReason(response) != "" {
			return model.TaskStatusFailure
		}
		return ""
	}
}

func responsesProgress(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess, model.TaskStatusFailure:
		return "100%"
	case model.TaskStatusInProgress:
		return "50%"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "0%"
	default:
		return ""
	}
}

func responsesMetadataValue(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	return common.Interface2String(metadata[key])
}

func responsesPromptFromRequest(request *dto.OpenAIResponsesRequest) string {
	if request == nil || len(request.Input) == 0 {
		return ""
	}
	var inputText string
	if err := json.Unmarshal(request.Input, &inputText); err == nil {
		return truncateResponsesTaskText(inputText)
	}
	var parts []string
	for _, input := range request.ParseInput() {
		if input.Text != "" {
			parts = append(parts, input.Text)
		}
	}
	if len(parts) > 0 {
		return truncateResponsesTaskText(strings.Join(parts, "\n"))
	}
	return truncateResponsesTaskText(string(request.Input))
}

func truncateResponsesTaskText(value string) string {
	const maxLen = 2000
	value = strings.TrimSpace(value)
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func responsesResultURLFromBody(responseBody []byte) string {
	if len(responseBody) == 0 {
		return ""
	}
	var root map[string]any
	if err := json.Unmarshal(responseBody, &root); err != nil {
		return ""
	}
	outputs, ok := root["output"].([]any)
	if !ok {
		return ""
	}
	for _, item := range outputs {
		output, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if url := common.Interface2String(output["url"]); url != "" {
			return url
		}
		if result := common.Interface2String(output["result"]); strings.HasPrefix(result, "http://") || strings.HasPrefix(result, "https://") {
			return result
		}
	}
	return ""
}

func applyResponsesTaskState(task *model.Task, response *dto.OpenAIResponsesResponse, responseBody []byte) model.TaskStatus {
	status := responsesTaskStatus(response)
	if status == "" {
		return ""
	}
	now := time.Now().Unix()
	task.Status = status
	if progress := responsesProgress(status); progress != "" {
		task.Progress = progress
	}
	if len(responseBody) > 0 {
		task.Data = json.RawMessage(responseBody)
	}
	switch status {
	case model.TaskStatusInProgress:
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		if task.StartTime == 0 {
			task.StartTime = task.SubmitTime
		}
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if resultURL := responsesResultURLFromBody(responseBody); resultURL != "" {
			task.PrivateData.ResultURL = resultURL
		}
	case model.TaskStatusFailure:
		if task.StartTime == 0 {
			task.StartTime = task.SubmitTime
		}
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = responsesFailureReason(response)
		if task.FailReason == "" {
			task.FailReason = "response task failed"
		}
	}
	return status
}

func RecordResponsesTaskSubmission(c *gin.Context, info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest, actualQuota int) {
	if c == nil || info == nil || request == nil {
		return
	}
	if !IsResponsesAsyncTaskRequest(request) && !c.GetBool("responses_async_task") {
		logger.LogWarn(c, fmt.Sprintf(
			"skip responses task submission: not async task, model=%s background=%s tools=%s",
			info.OriginModelName,
			string(request.Background),
			string(request.Tools),
		))
		return
	}
	response, responseBody, ok := capturedResponsesResponse(c)
	if !ok || response.ID == "" {
		logger.LogError(c, fmt.Sprintf(
			"skip responses task submission: response capture missing or empty id, model=%s captured=%t response_id=%q",
			info.OriginModelName,
			ok,
			func() string {
				if response == nil {
					return ""
				}
				return response.ID
			}(),
		))
		return
	}
	taskID := ResponsesTaskID(response.ID)
	if _, exists, err := model.GetByTaskId(info.UserId, taskID); err != nil {
		logger.LogWarn(c, fmt.Sprintf("load responses task %s failed: %s", response.ID, err.Error()))
		return
	} else if exists {
		logger.LogInfo(c, fmt.Sprintf("responses task %s already exists for response %s", taskID, response.ID))
		return
	}

	task := model.InitTask(constant.TaskPlatformResponses, info)
	task.TaskID = taskID
	task.Action = constant.TaskActionResponses
	task.Prompt = responsesPromptFromRequest(request)
	task.Properties.Input = truncateResponsesTaskText(string(request.Input))
	task.PrivateData.ResponseID = response.ID
	task.PrivateData.UpstreamTaskID = responsesMetadataValue(response.Metadata, "task_id")
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      info.PriceData.ModelRatio,
		OtherRatios:     info.PriceData.OtherRatios,
		OriginModelName: info.OriginModelName,
		PerCallBilling:  common.StringsContains(constant.TaskPricePatches, info.OriginModelName) || info.PriceData.UsePrice,
	}
	task.Quota = actualQuota
	if status := applyResponsesTaskState(task, response, responseBody); status == "" {
		task.Status = model.TaskStatusQueued
		task.Progress = "0%"
	}
	if err := task.Insert(); err != nil {
		logger.LogError(c, fmt.Sprintf("insert responses task %s failed: %s", response.ID, err.Error()))
		if task.Quota != 0 {
			RefundTaskQuota(c, task, "insert responses task failed")
		}
		return
	}
	logger.LogInfo(c, fmt.Sprintf(
		"inserted responses task: task_id=%s response_id=%s user_id=%d channel_id=%d quota=%d status=%s",
		task.TaskID,
		response.ID,
		task.UserId,
		task.ChannelId,
		task.Quota,
		task.Status,
	))
	logResponsesTaskConsumption(c, task, info)
	if task.Status == model.TaskStatusFailure && task.Quota != 0 && !task.PrivateData.Refunded {
		RefundTaskQuota(c, task, task.FailReason)
	} else if task.Status == model.TaskStatusSuccess {
		settleResponsesTaskOnSuccess(c, task, response)
	}
}

func logResponsesTaskConsumption(c *gin.Context, task *model.Task, info *relaycommon.RelayInfo) {
	if c == nil || task == nil || info == nil {
		return
	}
	other := taskBillingOther(task)
	other["is_task"] = true
	other["task_id"] = task.TaskID
	other["request_path"] = c.Request.URL.Path
	other["responses_async_task"] = true
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}

	logContent := fmt.Sprintf("操作 %s", task.Action)
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) || info.PriceData.UsePrice {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else if len(info.PriceData.OtherRatios) > 0 {
		var contents []string
		for key, ratio := range info.PriceData.OtherRatios {
			if ratio != 1.0 {
				contents = append(contents, fmt.Sprintf("%s: %.2f", key, ratio))
			}
		}
		if len(contents) > 0 {
			logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
		}
	}

	model.RecordConsumeLog(c, task.UserId, model.RecordConsumeLogParams{
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		TokenName: c.GetString("token_name"),
		Quota:     task.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     task.Group,
		Other:     other,
	})
	if task.Quota != 0 {
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, task.Quota)
		model.UpdateChannelUsedQuota(task.ChannelId, task.Quota)
	}
}

func settleResponsesTaskOnSuccess(ctx *gin.Context, task *model.Task, response *dto.OpenAIResponsesResponse) {
	if task == nil || response == nil {
		return
	}
	if task.PrivateData.BillingContext != nil && task.PrivateData.BillingContext.PerCallBilling {
		return
	}
	if response.Usage == nil || response.Usage.TotalTokens <= 0 {
		return
	}
	RecalculateTaskQuotaByTokens(ctx, task, response.Usage.TotalTokens)
}

func UpdateResponsesTaskFromFetch(c *gin.Context, info *relaycommon.RelayInfo, responseID string) {
	if c == nil || responseID == "" {
		return
	}
	response, responseBody, ok := capturedResponsesResponse(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	if userID == 0 && info != nil {
		userID = info.UserId
	}
	if userID == 0 {
		return
	}
	task, exists, err := model.GetByTaskId(userID, ResponsesTaskID(responseID))
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("load responses task %s failed: %s", responseID, err.Error()))
		return
	}
	if (!exists || task == nil) && response != nil && response.ID != "" && response.ID != responseID {
		task, exists, err = model.GetByTaskId(userID, ResponsesTaskID(response.ID))
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("load responses task %s failed: %s", response.ID, err.Error()))
			return
		}
	}
	if !exists || task == nil {
		logger.LogWarn(c, fmt.Sprintf("responses task not found for response_id=%s body_response_id=%s", responseID, response.ID))
		return
	}
	snap := task.Snapshot()
	status := applyResponsesTaskState(task, response, responseBody)
	if status == "" {
		return
	}
	isTerminal := status == model.TaskStatusSuccess || status == model.TaskStatusFailure
	shouldRefund := isTerminal && status == model.TaskStatusFailure && task.Quota != 0 && snap.Status != status && !task.PrivateData.Refunded
	shouldSettle := isTerminal && status == model.TaskStatusSuccess && snap.Status != status
	if isTerminal && snap.Status != status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("update responses task %s failed: %s", responseID, err.Error()))
			return
		}
		if !won {
			logger.LogWarn(c, fmt.Sprintf("responses task %s already transitioned, skip billing", responseID))
			return
		}
	} else if !snap.Equal(task.Snapshot()) {
		if _, err := task.UpdateWithStatus(snap.Status); err != nil {
			logger.LogError(c, fmt.Sprintf("update responses task %s failed: %s", responseID, err.Error()))
		}
	}
	if shouldRefund {
		RefundTaskQuota(c, task, task.FailReason)
	}
	if shouldSettle {
		settleResponsesTaskOnSuccess(c, task, response)
	}
}

func FailResponsesTaskFromFetch(c *gin.Context, info *relaycommon.RelayInfo, responseID string, reason string) {
	if c == nil || responseID == "" {
		return
	}
	userID := c.GetInt("id")
	if userID == 0 && info != nil {
		userID = info.UserId
	}
	if userID == 0 {
		return
	}
	task, exists, err := model.GetByTaskId(userID, ResponsesTaskID(responseID))
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("load responses task %s failed: %s", responseID, err.Error()))
		return
	}
	if !exists || task == nil {
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return
	}
	snap := task.Snapshot()
	now := time.Now().Unix()
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	if task.StartTime == 0 {
		task.StartTime = task.SubmitTime
	}
	if task.FinishTime == 0 {
		task.FinishTime = now
	}
	task.FailReason = strings.TrimSpace(reason)
	if task.FailReason == "" {
		task.FailReason = "response task query failed"
	}
	won, err := task.UpdateWithStatus(snap.Status)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("update responses task %s failed: %s", responseID, err.Error()))
		return
	}
	if !won {
		logger.LogWarn(c, fmt.Sprintf("responses task %s already transitioned, skip billing", responseID))
		return
	}
	if task.Quota != 0 && !task.PrivateData.Refunded {
		RefundTaskQuota(c, task, task.FailReason)
	}
}
