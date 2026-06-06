package service

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
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

func CaptureResponsesResponse(c *gin.Context, response *dto.OpenAIResponsesResponse, responseBody []byte) {
	if c == nil || response == nil {
		return
	}
	c.Set(responsesResponseContextKey, response)
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
	if c == nil || info == nil || request == nil || !responsesRequestBackground(request) {
		return
	}
	response, responseBody, ok := capturedResponsesResponse(c)
	if !ok || response.ID == "" {
		return
	}
	taskID := ResponsesTaskID(response.ID)
	if _, exists, err := model.GetByTaskId(info.UserId, taskID); err != nil {
		logger.LogWarn(c, fmt.Sprintf("load responses task %s failed: %s", response.ID, err.Error()))
		return
	} else if exists {
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
	if task.Status == model.TaskStatusFailure && task.Quota != 0 {
		RefundTaskQuota(c, task, task.FailReason)
	} else if task.Status == model.TaskStatusSuccess {
		settleResponsesTaskOnSuccess(c, task, response)
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
	if !exists || task == nil {
		return
	}
	snap := task.Snapshot()
	status := applyResponsesTaskState(task, response, responseBody)
	if status == "" {
		return
	}
	isTerminal := status == model.TaskStatusSuccess || status == model.TaskStatusFailure
	shouldRefund := isTerminal && status == model.TaskStatusFailure && task.Quota != 0 && snap.Status != status
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
	if task.Quota != 0 {
		RefundTaskQuota(c, task, task.FailReason)
	}
}
