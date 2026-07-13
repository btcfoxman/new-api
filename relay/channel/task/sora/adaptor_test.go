package sora

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

func TestDashScopeHappyHorseRequestUsesNestedPromptForSoraChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model": "happyhorse-1.0-r2v",
		"input": {
			"prompt": "move the person from image one into image two",
			"media": [
				{"type": "reference_image", "url": "https://example.com/one.png"},
				{"type": "reference_image", "url": "https://example.com/two.png"}
			]
		},
		"parameters": {
			"resolution": "1080P",
			"duration": 5,
			"ratio": "16:9"
		}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "happyhorse-1.0",
		},
	}

	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		t.Fatalf("GetTaskRequest() error = %v", err)
	}
	if req.Prompt != "move the person from image one into image two" {
		t.Fatalf("prompt = %q", req.Prompt)
	}
	if req.Mode != "r2v" {
		t.Fatalf("mode = %q, want r2v", req.Mode)
	}
	if req.Duration != 5 {
		t.Fatalf("duration = %d, want 5", req.Duration)
	}
	if req.Size != "1080P" {
		t.Fatalf("size = %q, want 1080P", req.Size)
	}
	if info.Action != constant.TaskActionReferenceGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionReferenceGenerate)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["model"] != "happyhorse-1.0" {
		t.Fatalf("model = %v, want happyhorse-1.0", payload["model"])
	}
	if payload["prompt"] != "move the person from image one into image two" {
		t.Fatalf("prompt = %v", payload["prompt"])
	}
	if payload["mode"] != "r2v" {
		t.Fatalf("mode = %v, want r2v", payload["mode"])
	}
	input, ok := payload["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("input = %#v", payload["input"])
	}
	media, ok := input["media"].([]interface{})
	if !ok || len(media) != 2 {
		t.Fatalf("media = %#v", input["media"])
	}
}

func TestBuildRequestBodyRewritesURLEncodedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := "model=sora-2&prompt=test+prompt&seconds=12&size=720x1280"
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "Application/X-WWW-Form-Urlencoded; charset=UTF-8")
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "sora-2-stable"},
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}

	encodedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	values, err := url.ParseQuery(string(encodedBody))
	if err != nil {
		t.Fatalf("url.ParseQuery() error = %v", err)
	}

	if got := values.Get("model"); got != "sora-2-stable" {
		t.Fatalf("model = %q, want %q", got, "sora-2-stable")
	}
	if got := values.Get("prompt"); got != "test prompt" {
		t.Fatalf("prompt = %q, want %q", got, "test prompt")
	}
	if got := values.Get("seconds"); got != "12" {
		t.Fatalf("seconds = %q, want %q", got, "12")
	}
}

func TestBuildRequestBodyAddsURLEncodedModelWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := "prompt=test+prompt&size=720x1280"
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", gin.MIMEPOSTForm)
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "sora-2-stable"},
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}

	encodedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	values, err := url.ParseQuery(string(encodedBody))
	if err != nil {
		t.Fatalf("url.ParseQuery() error = %v", err)
	}

	if got := values.Get("model"); got != "sora-2-stable" {
		t.Fatalf("model = %q, want %q", got, "sora-2-stable")
	}
	if got := values.Get("size"); got != "720x1280" {
		t.Fatalf("size = %q, want %q", got, "720x1280")
	}
}

func TestBuildRequestBodyUsesOfficialModelMappingForURLEncodedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := "model=sora-2&prompt=test+prompt&seconds=12&size=720x1280"
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req
	c.Set("model_mapping", `{"sora-2":"sora-2-official"}`)

	info := &relaycommon.RelayInfo{
		OriginModelName: "sora-2",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "sora-2"},
	}
	if err := relayhelper.ModelMappedHelper(c, info, nil); err != nil {
		t.Fatalf("ModelMappedHelper() error = %v", err)
	}
	if got := info.UpstreamModelName; got != "sora-2-official" {
		t.Fatalf("UpstreamModelName = %q, want %q", got, "sora-2-official")
	}

	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	encodedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	values, err := url.ParseQuery(string(encodedBody))
	if err != nil {
		t.Fatalf("url.ParseQuery() error = %v", err)
	}
	if got := values.Get("model"); got != "sora-2-official" {
		t.Fatalf("model = %q, want %q", got, "sora-2-official")
	}
}

func TestBuildRequestBodyPreservesReferenceImagesArrayForJSONRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := `{"model":"grok-imagine-video-1.5-preview","prompt":"test","duration":20,"reference_images":["https://example.com/1.png","https://example.com/2.png"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video-1.5-preview-cyuapi"},
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}

	rewrittenBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rewrittenBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body=%s", err, rewrittenBody)
	}
	if got := payload["model"]; got != "grok-imagine-video-1.5-preview-cyuapi" {
		t.Fatalf("model = %q, want mapped model", got)
	}
	refs, ok := payload["reference_images"].([]any)
	if !ok {
		t.Fatalf("reference_images type = %T, want []any; body=%s", payload["reference_images"], rewrittenBody)
	}
	if len(refs) != 2 || refs[0] != "https://example.com/1.png" || refs[1] != "https://example.com/2.png" {
		t.Fatalf("reference_images = %#v", refs)
	}
}

func TestEstimateBillingUsesParametersDurationWhenTopLevelDurationMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0",
		Mode:   "r2v",
		Prompt: "turn reference images into a video",
		Parameters: map[string]interface{}{
			"duration":   float64(12),
			"resolution": "720P",
		},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	if got := ratios["seconds"]; got != 12 {
		t.Fatalf("seconds ratio = %v, want 12", got)
	}
}

func TestEstimateBillingUsesInlineContentDurationWhenTopLevelDurationMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-fast-260128-sdapi",
		Mode:   "reference_images",
		Prompt: "reference based character video",
		Content: []map[string]interface{}{
			{
				"type": "text",
				"text": "reference based character video --ratio 9:16 --dur 10",
			},
			{
				"type":      "image_url",
				"image_url": "https://example.com/ref.png",
			},
		},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	if got := ratios["seconds"]; got != 10 {
		t.Fatalf("seconds ratio = %v, want 10", got)
	}
}

func TestEstimateBillingPrefersTopLevelSecondsOverParametersDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:   "sora-2",
		Prompt:  "test prompt",
		Seconds: "8",
		Parameters: map[string]interface{}{
			"duration": float64(12),
		},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	if got := ratios["seconds"]; got != 8 {
		t.Fatalf("seconds ratio = %v, want 8", got)
	}
}

func TestEstimateBillingPrefersParametersDurationOverInlineContentDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-fast-260128-sdapi",
		Prompt: "reference based character video",
		Parameters: map[string]interface{}{
			"duration": float64(12),
		},
		Content: []map[string]interface{}{
			{
				"type": "text",
				"text": "reference based character video --duration=10",
			},
		},
	})

	ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{})

	if got := ratios["seconds"]; got != 12 {
		t.Fatalf("seconds ratio = %v, want 12", got)
	}
}
