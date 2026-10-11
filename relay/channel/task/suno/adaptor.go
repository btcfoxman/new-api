package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
}

// ParseTaskResult is not used for Suno tasks.
// Suno polling uses a dedicated batch-fetch path (service.UpdateSunoTasks) that
// receives dto.TaskResponse[[]dto.SunoDataResponse] from the upstream /fetch API.
// This differs from the per-task polling used by video adaptors.
func (a *TaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, fmt.Errorf("suno uses batch polling via UpdateSunoTasks, ParseTaskResult is not applicable")
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	value, ok := c.Get("task_request")
	if !ok {
		return nil
	}
	sunoRequest, ok := value.(*dto.SunoSubmitReq)
	if !ok || sunoRequest == nil {
		return nil
	}
	modelName := resolveSunoBillingModel(sunoRequest)
	credits := resolveSunoDynamicCredits(sunoRequest, modelName)
	if credits <= 0 {
		return nil
	}
	return map[string]float64{"credits": float64(credits)}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	action := strings.ToUpper(c.Param("action"))

	var sunoRequest *dto.SunoSubmitReq
	err := common.UnmarshalBodyReusable(c, &sunoRequest)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}
	err = actionValidate(c, sunoRequest, action)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}

	//if sunoRequest.ContinueClipId != "" {
	//	if sunoRequest.TaskID == "" {
	//		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_request", http.StatusBadRequest)
	//		return
	//	}
	//	info.OriginTaskID = sunoRequest.TaskID
	//}

	info.Action = action
	if action == constant.SunoActionMusic {
		c.Set("task_billing_model", resolveSunoBillingModel(sunoRequest))
	}
	c.Set("task_request", sunoRequest)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	actionPath := strings.ToLower(info.Action)
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, "/suno/submit/"+actionPath)
	return fullRequestURL, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	sunoRequest, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("task_request not found in context")
	}
	data, err := common.Marshal(sunoRequest)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	var sunoResponse dto.TaskResponse[string]
	err = common.Unmarshal(responseBody, &sunoResponse)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if !sunoResponse.IsSuccess() {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", sunoResponse.Message), sunoResponse.Code, http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 替换上游 ID 返回给客户端
	publicResponse := dto.TaskResponse[string]{
		Code:    sunoResponse.Code,
		Message: sunoResponse.Message,
		Data:    info.PublicTaskID,
	}
	c.JSON(http.StatusOK, publicResponse)

	return sunoResponse.Data, nil, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	requestUrl := fmt.Sprintf("%s/suno/fetch", baseUrl)
	byteBody, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(byteBody))
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task error: %v", err))
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func actionValidate(c *gin.Context, sunoRequest *dto.SunoSubmitReq, action string) (err error) {
	switch action {
	case constant.SunoActionMusic:
		if sunoRequest.Model == "" {
			sunoRequest.Model = "suno_music"
		}
		hasExplicitMusicModel := sunoRequest.UpstreamModel != "" ||
			sunoRequest.MusicModel != "" ||
			sunoRequest.Engine != "" ||
			hasSunoModelValue(sunoRequest.ModelID) ||
			hasSunoModelValue(sunoRequest.ModelIDCamel) ||
			sunoRequest.Mv != "" ||
			normalizeSunoMusicBillingModel(sunoRequest.Model) != ""
		if !hasExplicitMusicModel {
			sunoRequest.UpstreamModel = "suno-v5.5"
		}
	case constant.SunoActionLyrics:
		if sunoRequest.Model == "" {
			sunoRequest.Model = "suno_lyrics"
		}
		if sunoRequest.Prompt == "" {
			err = fmt.Errorf("prompt_empty")
			return
		}
	default:
		err = fmt.Errorf("invalid_action")
	}
	return
}

func resolveSunoBillingModel(sunoRequest *dto.SunoSubmitReq) string {
	if sunoRequest == nil {
		return "suno-v5.5"
	}
	for _, value := range []interface{}{
		sunoRequest.UpstreamModel,
		sunoRequest.MusicModel,
		sunoRequest.Engine,
		sunoRequest.ModelID,
		sunoRequest.ModelIDCamel,
		sunoRequest.Mv,
		sunoRequest.Model,
	} {
		if modelName := normalizeSunoMusicBillingModel(value); modelName != "" {
			return modelName
		}
	}
	return "suno-v5.5"
}

func normalizeSunoMusicBillingModel(value interface{}) string {
	if !hasSunoModelValue(value) {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" || normalized == "music" || normalized == "suno-music" {
		return ""
	}
	switch normalized {
	case "suno-v6":
		return "suno-v6"
	case "suno-v5.5", "suno-v55", "suno-5.5", "suno-55", "v5.5", "v55", "chirp-fenix", "128":
		return "suno-v5.5"
	case "seed-audio", "seed-audio-1.0", "seed-audio-1-0", "126":
		return "seed-audio-1.0"
	case "minimax-speech", "minimax-speech-2.8-hd", "minimax-speech-2-8-hd", "speech-2.8-hd", "speech-2-8-hd", "127":
		return "minimax-speech-2.8-hd"
	case "lyria", "lyria-3-pro", "lyria-3-pro-preview", "129":
		return "lyria-3-pro"
	case "suno", "suno-v5", "suno-5", "v5", "chirp-v5", "chirp-v5.0", "chirp-crow", "85":
		return "suno-v5"
	case "minimax", "minimax-music", "minimax-music-v2.6", "minimax-music-v2-6", "minimax-music-2.6", "105":
		return "minimax-music-v2.6"
	case "sonauto", "sonauto-v3", "82":
		return "sonauto-v3"
	case "mureka", "mureka-v9", "mureka-9", "114":
		return "mureka-v9"
	default:
		return ""
	}
}

func hasSunoModelValue(value interface{}) bool {
	if value == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(value)) != ""
}

func resolveSunoDynamicCredits(sunoRequest *dto.SunoSubmitReq, modelName string) int {
	if sunoRequest == nil {
		return 0
	}
	if modelName != "seed-audio-1.0" && modelName != "minimax-speech-2.8-hd" {
		return 0
	}
	prompt := strings.TrimSpace(sunoRequest.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(sunoRequest.GptDescriptionPrompt)
	}
	charCount := utf8.RuneCountInString(prompt)
	if charCount < 1 {
		charCount = 1
	}
	if modelName == "seed-audio-1.0" {
		return ((charCount + 14) / 15) * 2
	}
	credits := (charCount / 100) * 10
	if credits < 10 {
		return 10
	}
	return credits
}
