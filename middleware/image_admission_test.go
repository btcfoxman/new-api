package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func imageAdmissionRequest(path, body string) *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	return context
}

func TestImageSyncAdmissionClassificationRestoresRequestBody(t *testing.T) {
	tests := []struct {
		name, path, body string
		want             bool
	}{
		{"generation", "/v1/images/generations", `{}`, true},
		{"edit", "/v1/images/edits", `{}`, true},
		{"async image", "/v1/responses", `{"background":true,"stream":false,"tools":[{"type":"image_generation"}]}`, false},
		{"sync image", "/v1/responses", `{"background":false,"tools":[{"type":"image_generation"}]}`, true},
		{"image tool choice", "/v1/responses", `{"tool_choice":"image_generation"}`, true},
		{"chat image model", "/v1/chat/completions", `{"model":"gpt-image-2","messages":[]}`, true},
		{"chat text model", "/v1/chat/completions", `{"model":"gpt-6-sol","messages":[]}`, false},
		{"background image model", "/v1/responses", `{"model":"gpt-image-2","background":true,"input":"a blue circle"}`, false},
		{"text", "/v1/responses", `{"background":false,"tool_choice":"auto"}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := imageAdmissionRequest(test.path, test.body)
			if got := isSynchronousImageRequest(context); got != test.want {
				t.Fatalf("isSynchronousImageRequest = %t, want %t", got, test.want)
			}
			body, err := io.ReadAll(context.Request.Body)
			if err != nil || string(body) != test.body {
				t.Fatalf("request body after classification = %q, err %v", body, err)
			}
		})
	}
}

func TestImageSyncAdmissionLocalCapacity(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() { common.RedisEnabled = oldRedisEnabled }()
	context := imageAdmissionRequest("/v1/images/generations", `{}`).Request.Context()
	release, _, err := acquireImageSyncSlot(context, "first", 1)
	if err != nil || release == nil {
		t.Fatalf("first admission release_nil=%t, err=%v", release == nil, err)
	}
	second, _, err := acquireImageSyncSlot(context, "second", 1)
	if err != nil || second != nil {
		t.Fatalf("second admission release_nil=%t, err=%v", second == nil, err)
	}
	release()
	third, _, err := acquireImageSyncSlot(context, "third", 1)
	if err != nil || third == nil {
		t.Fatalf("slot was not released release_nil=%t, err=%v", third == nil, err)
	}
	third()
}
