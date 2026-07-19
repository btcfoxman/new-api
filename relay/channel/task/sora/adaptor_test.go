package sora

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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

func TestDashScopeHappyHorseRequestAcceptsTopLevelPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model": "happyhorse-1.0-r2v",
		"prompt": "top-level prompt",
		"input": {
			"media": [{"type": "reference_image", "url": "https://example.com/one.png"}]
		},
		"parameters": {"duration": 5}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "happyhorse-1.0"},
	}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		t.Fatalf("GetTaskRequest() error = %v", err)
	}
	if req.Prompt != "top-level prompt" {
		t.Fatalf("prompt = %q", req.Prompt)
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
	input, ok := payload["input"].(map[string]interface{})
	if !ok || input["prompt"] != "top-level prompt" {
		t.Fatalf("input = %#v", payload["input"])
	}
}

func TestDashScopeHappyHorseCreateReturnsOfficialResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", nil)
	upstreamResponse := httptest.NewRecorder()
	upstreamResponse.Code = http.StatusOK
	upstreamResponse.Body.WriteString(`{
		"id": "upstream-video-id",
		"object": "video",
		"model": "happyhorse-1.0-r2v",
		"status": "queued",
		"progress": 0,
		"created_at": 1783950000,
		"request_id": "upstream-request-id",
		"output": {
			"task_id": "upstream-video-id",
			"task_status": "SUCCEEDED"
		}
	}`)

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	upstreamID, _, taskErr := adaptor.DoResponse(c, upstreamResponse.Result(), info)
	if taskErr != nil {
		t.Fatalf("DoResponse() error = %v", taskErr)
	}
	if upstreamID != "upstream-video-id" {
		t.Fatalf("upstreamID = %q", upstreamID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	output, ok := payload["output"].(map[string]interface{})
	if !ok {
		t.Fatalf("output = %#v", payload["output"])
	}
	if output["task_id"] != "task_public" || output["task_status"] != "PENDING" {
		t.Fatalf("output = %#v", output)
	}
	if payload["request_id"] != "upstream-request-id" {
		t.Fatalf("request_id = %#v", payload["request_id"])
	}
	if payload["id"] != "task_public" || payload["task_id"] != "task_public" {
		t.Fatalf("public task ids are missing: %#v", payload)
	}
	if payload["object"] != "video" || payload["status"] != "queued" {
		t.Fatalf("Sora compatibility fields are missing: %#v", payload)
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

func TestBuildRequestBodyReferenceModeOverridesConflictingFrameFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{
		"model":"doubao-seedance-2-0-fast-260128",
		"mode":"reference_material",
		"prompt":"keep the subject consistent",
		"image":"https://example.com/one.png",
		"image_url":"https://example.com/one.png",
		"image_urls":["https://example.com/one.png","https://example.com/two.png"],
		"reference_images":["https://example.com/one.png","https://example.com/two.png"],
		"first_frame_url":"https://example.com/one.png",
		"end_frame_url":"https://example.com/two.png",
		"last_frame_url":"https://example.com/three.png",
		"content":[
			{"type":"text","text":"keep the subject consistent"},
			{"type":"image_url","image_url":{"url":"https://example.com/four.png"},"role":"first-frame"},
			{"type":"video_url","video_url":{"url":"https://example.com/reference.mp4"},"role":"reference_video"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seedance-2-0-fast-260128-lyapi"},
	}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
	}
	if info.Action != constant.TaskActionReferenceGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionReferenceGenerate)
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
	if payload["model"] != "doubao-seedance-2-0-fast-260128-lyapi" {
		t.Fatalf("model = %#v", payload["model"])
	}
	for _, field := range []string{"first_frame_url", "end_frame_url", "last_frame_url"} {
		if _, exists := payload[field]; exists {
			t.Fatalf("reference mode must remove %s: %s", field, rewrittenBody)
		}
	}
	if payload["image"] != "https://example.com/one.png" || payload["image_url"] != "https://example.com/one.png" {
		t.Fatalf("non-frame reference aliases were not preserved: %s", rewrittenBody)
	}
	refs, ok := payload["reference_images"].([]any)
	if !ok {
		t.Fatalf("reference_images type = %T, body=%s", payload["reference_images"], rewrittenBody)
	}
	if len(refs) != 3 || refs[2] != "https://example.com/three.png" {
		t.Fatalf("reference_images = %#v, want the unique frame promoted", refs)
	}
	content, ok := payload["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("content = %#v", payload["content"])
	}
	imageItem, ok := content[1].(map[string]any)
	if !ok || imageItem["role"] != "reference_image" {
		t.Fatalf("image content role = %#v", content[1])
	}
	videoItem, ok := content[2].(map[string]any)
	if !ok || videoItem["role"] != "reference_video" {
		t.Fatalf("video content role = %#v", content[2])
	}
}

func TestBuildRequestBodyKeepsFrameFieldsOutsideReferenceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{
		"model":"doubao-seedance-2-0-fast-260128",
		"mode":"first_last",
		"prompt":"transition between frames",
		"first_frame_url":"https://example.com/first.png",
		"last_frame_url":"https://example.com/last.png",
		"content":[{"type":"image_url","image_url":{"url":"https://example.com/first.png"},"role":"first_frame"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	rewrittenBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(rewrittenBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["first_frame_url"] != "https://example.com/first.png" || payload["last_frame_url"] != "https://example.com/last.png" {
		t.Fatalf("frame fields changed outside reference mode: %s", rewrittenBody)
	}
	content := payload["content"].([]any)
	if content[0].(map[string]any)["role"] != "first_frame" {
		t.Fatalf("frame role changed outside reference mode: %s", rewrittenBody)
	}
}

func TestBuildRequestBodyReferenceModeNormalizesURLEncodedFrameFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	values := url.Values{
		"model":           {"doubao-seedance"},
		"mode":            {"reference_material"},
		"prompt":          {"reference video"},
		"image_urls":      {"https://example.com/one.png"},
		"first_frame_url": {"https://example.com/one.png"},
		"last_frame_url":  {"https://example.com/two.png"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", gin.MIMEPOSTForm)
	c.Request = req

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	rewrittenBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	rewrittenValues, err := url.ParseQuery(string(rewrittenBody))
	if err != nil {
		t.Fatalf("url.ParseQuery() error = %v", err)
	}
	if rewrittenValues.Has("first_frame_url") || rewrittenValues.Has("last_frame_url") {
		t.Fatalf("frame fields were not removed: %s", rewrittenBody)
	}
	if got := rewrittenValues["reference_images"]; len(got) != 1 || got[0] != "https://example.com/two.png" {
		t.Fatalf("reference_images = %#v", got)
	}
}

func TestBuildRequestBodyReferenceModeNormalizesMultipartFrameFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var requestBody bytes.Buffer
	requestWriter := multipart.NewWriter(&requestBody)
	for field, value := range map[string]string{
		"model":            "doubao-seedance",
		"mode":             "reference_material",
		"prompt":           "reference video",
		"image_urls":       "https://example.com/one.png",
		"first_frame_url":  "https://example.com/one.png",
		"end_frame_url":    "https://example.com/two.png",
		"unrelated_option": "preserved",
	} {
		if err := requestWriter.WriteField(field, value); err != nil {
			t.Fatalf("WriteField(%q) error = %v", field, err)
		}
	}
	if err := requestWriter.Close(); err != nil {
		t.Fatalf("requestWriter.Close() error = %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(requestBody.Bytes()))
	req.Header.Set("Content-Type", requestWriter.FormDataContentType())
	c.Request = req

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	rewrittenBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	_, parameters, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	rewrittenForm, err := multipart.NewReader(bytes.NewReader(rewrittenBody), parameters["boundary"]).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("ReadForm() error = %v", err)
	}
	defer rewrittenForm.RemoveAll()

	if _, exists := rewrittenForm.Value["first_frame_url"]; exists {
		t.Fatalf("first_frame_url was not removed: %#v", rewrittenForm.Value)
	}
	if _, exists := rewrittenForm.Value["end_frame_url"]; exists {
		t.Fatalf("end_frame_url was not removed: %#v", rewrittenForm.Value)
	}
	if got := rewrittenForm.Value["reference_images"]; len(got) != 1 || got[0] != "https://example.com/two.png" {
		t.Fatalf("reference_images = %#v", got)
	}
	if got := rewrittenForm.Value["unrelated_option"]; len(got) != 1 || got[0] != "preserved" {
		t.Fatalf("unrelated_option = %#v", got)
	}
}

func TestBuildRequestBodyDoesNotNormalizeRemixPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{
		"model":"doubao-seedance",
		"mode":"reference_material",
		"prompt":"remix this video",
		"first_frame_url":"https://example.com/first.png",
		"content":[{"type":"image_url","image_url":{"url":"https://example.com/first.png"},"role":"first_frame"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/videos/task_origin/remix", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionRemix},
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"},
	}
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody() error = %v", err)
	}
	rewrittenBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(rewrittenBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["first_frame_url"] != "https://example.com/first.png" {
		t.Fatalf("remix frame field changed: %s", rewrittenBody)
	}
	content := payload["content"].([]any)
	if content[0].(map[string]any)["role"] != "first_frame" {
		t.Fatalf("remix content role changed: %s", rewrittenBody)
	}
}

func TestValidateRequestAndSetActionKeepsLegacyHeuristicsOutsideReferenceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		mode       string
		mediaField string
		wantAction string
	}{
		{
			name:       "text request",
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:       "image request with t2v mode retains image heuristic",
			mode:       "t2v",
			mediaField: `,"image_url":"https://example.com/input.png"`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "first last mode retains existing image heuristic",
			mode:       "first_last",
			mediaField: `,"reference_images":["https://example.com/first.png","https://example.com/last.png"]`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "unknown mode retains existing image heuristic",
			mode:       "provider_specific",
			mediaField: `,"image":"https://example.com/input.png"`,
			wantAction: constant.TaskActionGenerate,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			modeField := ""
			if tc.mode != "" {
				modeField = `,"mode":"` + tc.mode + `"`
			}
			body := `{"model":"test-model","prompt":"test prompt"` + modeField + tc.mediaField + `}`
			req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			if taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info); taskErr != nil {
				t.Fatalf("ValidateRequestAndSetAction() error = %v", taskErr)
			}
			if info.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", info.Action, tc.wantAction)
			}
		})
	}
}

func TestReferenceVideoModeAliases(t *testing.T) {
	for _, mode := range []string{
		"reference_material",
		" Reference-Material ",
		"r2v",
		"reference-video",
		"multi reference",
	} {
		if !isReferenceVideoMode(mode) {
			t.Fatalf("mode %q was not recognized as reference video mode", mode)
		}
	}
	for _, mode := range []string{"", "t2v", "i2v", "first_last", "provider_specific"} {
		if isReferenceVideoMode(mode) {
			t.Fatalf("mode %q was incorrectly recognized as reference video mode", mode)
		}
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
