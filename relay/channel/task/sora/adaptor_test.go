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
