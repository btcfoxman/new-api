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
