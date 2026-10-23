package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewaycommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestKlingVideoModeValidationPreservesExplicitModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"", "t2v", "i2v", "i2v_first_last", "reference_images", "reference_material", "unknown_mode"} {
		for _, direct := range []bool{false, true} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			encoded, err := gatewaycommon.Marshal(map[string]any{"model": "kling-4.0", "prompt": "A product shot", "mode": mode})
			if err != nil {
				t.Fatal(err)
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(string(encoded)))
			c.Request.Header.Set("Content-Type", "application/json")
			info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
			var invalid bool
			if direct {
				invalid = ValidateMultipartDirect(c, info) != nil
			} else {
				invalid = ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate) != nil
			}
			if invalid != (mode == "unknown_mode") {
				t.Fatalf("mode=%q direct=%v invalid=%v", mode, direct, invalid)
			}
			if !invalid {
				req, err := GetTaskRequest(c)
				if err != nil || req.Mode != mode {
					t.Fatalf("mode lost: got=%q want=%q err=%v", req.Mode, mode, err)
				}
			}
		}
	}
	if err := validateKlingVideoMode(TaskSubmitReq{Model: "seedance-2.0", Mode: "provider_specific"}); err != nil {
		t.Fatalf("non-Kling mode validation changed: %v", err)
	}
}

func TestKlingFourContentsAndSettings(t *testing.T) {
	var req TaskSubmitReq
	if err := gatewaycommon.Unmarshal([]byte(`{
		"model":"kling-4.0", "duration":30, "seconds":"30", "contents":[
			{"type":"prompt","text":"A product shot"},
			{"type":"reference_image","url":"https://example.com/image.jpg"},
			{"type":"reference_video","url":"https://example.com/video.mp4"}
		], "settings":{"duration":8,"resolution":"720p","aspect_ratio":"16:9","audio":"native"}, "options":{}
	}`), &req); err != nil {
		t.Fatal(err)
	}
	req.FillPromptFromContent()
	if req.Prompt != "A product shot" || req.Duration != 8 || req.Seconds != "" {
		t.Fatalf("prompt/duration lost: %+v", req)
	}
	if len(req.Content) != 3 || req.Content[1]["type"] != "image_url" || req.Content[2]["type"] != "video_url" {
		t.Fatalf("media conversion failed: %+v", req.Content)
	}
	if req.Metadata["ratio"] != "16:9" || req.Metadata["generate_audio"] != true {
		t.Fatalf("settings conversion failed: %+v", req.Metadata)
	}
	req.FillPromptFromContent()
	if len(req.Content) != 3 {
		t.Fatalf("repeated normalization duplicated media: %+v", req.Content)
	}
}

func TestKlingFourPreservesExplicitZeroAndFrameRoles(t *testing.T) {
	var req TaskSubmitReq
	if err := gatewaycommon.Unmarshal([]byte(`{
		"model":"kling-4.0-1080p", "contents":[
			{"type":"prompt","text":"A product shot"},
			{"type":"FIRST-FRAME","url":"https://example.com/start.jpg"},
			{"type":"last_frame","url":"https://example.com/end.jpg"}
		], "settings":{"duration":"4","seed":0,"audio":" false "},
		"options":{"watermark":false}
	}`), &req); err != nil {
		t.Fatal(err)
	}
	req.FillPromptFromContent()
	if req.Duration != 4 || req.Metadata["generate_audio"] != false || req.Metadata["seed"] != float64(0) || req.Options["watermark"] != false {
		t.Fatalf("settings lost explicit values: %+v", req)
	}
	if req.Content[1]["role"] != "first_frame" || req.Content[2]["role"] != "last_frame" {
		t.Fatalf("frame roles lost: %+v", req.Content)
	}
}
