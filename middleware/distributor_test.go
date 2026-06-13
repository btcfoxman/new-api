package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestGetModelRequestReadsURLEncodedVideoModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	form := url.Values{}
	form.Set("model", "sora-2")
	form.Set("prompt", "test prompt")
	form.Set("seconds", "12")
	form.Set("size", "720x1280")
	form.Set("image", "https://example.com/reference.jpg?x=1&y=2")
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "Application/X-WWW-Form-Urlencoded; charset=UTF-8")
	c.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("shouldSelectChannel = false, want true")
	}
	if modelRequest.Model != "sora-2" {
		t.Fatalf("model = %q, want %q", modelRequest.Model, "sora-2")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoSubmit {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoSubmit)
	}
}

func TestGetModelRequestReadsAliOfficialHappyHorseModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model": "happyhorse-1.0-t2v",
		"input": {"prompt": "horse in neon city"},
		"parameters": {"resolution": "720P", "duration": 5}
	}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("shouldSelectChannel = false, want true")
	}
	if modelRequest.Model != "happyhorse-1.0-t2v" {
		t.Fatalf("model = %q, want %q", modelRequest.Model, "happyhorse-1.0-t2v")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoSubmit {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoSubmit)
	}
}

func TestGetModelRequestReadsDoubaoOfficialVideoModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model": "gemini-omni",
		"content": [
			{"type": "text", "text": "a fashion product video"},
			{"type": "image_url", "image_url": {"url": "https://example.com/ref.png"}}
		],
		"mode": "r2v",
		"duration": 8
	}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("shouldSelectChannel = false, want true")
	}
	if modelRequest.Model != "gemini-omni" {
		t.Fatalf("model = %q, want %q", modelRequest.Model, "gemini-omni")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoSubmit {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoSubmit)
	}
}

func TestGetModelRequestReadsDoubaoPureOfficialVideoModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(http.MethodPost, "/oapi/v3/contents/generations/tasks", strings.NewReader(`{
		"model": "gemini-omni",
		"content": [
			{"type": "text", "text": "a fashion product video"},
			{"type": "image_url", "image_url": {"url": "https://example.com/ref.png"}}
		],
		"mode": "r2v",
		"duration": 8
	}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("shouldSelectChannel = false, want true")
	}
	if modelRequest.Model != "gemini-omni" {
		t.Fatalf("model = %q, want %q", modelRequest.Model, "gemini-omni")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoSubmit {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoSubmit)
	}
}

func TestGetModelRequestAliOfficialHappyHorseTaskFetchDoesNotSelectChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_123", nil)

	_, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if shouldSelectChannel {
		t.Fatal("shouldSelectChannel = true, want false")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoFetchByID {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoFetchByID)
	}
}

func TestGetModelRequestDoubaoPureOfficialTaskFetchDoesNotSelectChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/oapi/v3/contents/generations/tasks/task_123", nil)

	_, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if shouldSelectChannel {
		t.Fatal("shouldSelectChannel = true, want false")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoFetchByID {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoFetchByID)
	}
}

func TestGetModelRequestDoubaoOfficialTaskFetchDoesNotSelectChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_123", nil)

	_, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if shouldSelectChannel {
		t.Fatal("shouldSelectChannel = true, want false")
	}
	if relayMode := c.GetInt("relay_mode"); relayMode != relayconstant.RelayModeVideoFetchByID {
		t.Fatalf("relay_mode = %d, want %d", relayMode, relayconstant.RelayModeVideoFetchByID)
	}
}
