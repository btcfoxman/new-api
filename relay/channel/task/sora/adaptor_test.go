package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

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
