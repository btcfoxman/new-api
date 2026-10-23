package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKlingNativeRequestNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, path, body, model, role string
		duration                      int
	}{
		{"text defaults", "/text-to-video/kling-4.0", `{"prompt":"Camera follows a runner"}`, "kling-4.0-1080p", "", 10},
		{"text 720", "/text-to-video/kling-4.0", `{"prompt":"Camera follows a runner","settings":{"resolution":"720p","duration":4,"aspect_ratio":"9:16"},"options":{"watermark_info":{"enabled":false}}}`, "kling-4.0", "", 4},
		{"image", "/image-to-video/kling-4.0", `{"contents":[{"type":"prompt","text":"Camera pulls back"},{"type":"first_frame","url":"https://example.com/start.png"},{"type":"last_frame","url":"https://example.com/end.png"}],"settings":{"duration":30}}`, "kling-4.0-1080p", "first_frame", 30},
		{"omni image", "/omni-video/kling-4.0", `{"contents":[{"type":"prompt","text":"Match this style"},{"type":"refer_image","url":"https://example.com/style.png"}]}`, "kling-4.0-1080p", "reference_image", 10},
		{"omni video", "/omni-video/kling-4.0", `{"contents":[{"type":"prompt","text":"Follow the movement"},{"type":"feature_video","url":"https://example.com/reference.mp4"}]}`, "kling-4.0-1080p", "reference_video", 10},
		{"omni last frame", "/omni-video/kling-4.0", `{"contents":[{"type":"prompt","text":"End on this scene"},{"type":"last_frame","url":"https://example.com/end.png"}]}`, "kling-4.0-1080p", "last_frame", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedTool := "omni_ref_video"
			if tc.path == "/image-to-video/kling-4.0" {
				expectedTool = "image_to_video"
			} else if tc.path == "/text-to-video/kling-4.0" {
				expectedTool = "text_to_video"
			}
			router := gin.New()
			router.POST(tc.path, KlingNativeRequestConvert(), func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				require.Equal(t, "/v1/video/generations", c.Request.URL.Path)
				var request relaycommon.TaskSubmitReq
				require.NoError(t, common.UnmarshalBodyReusable(c, &request))
				if tc.role == "first_frame" || tc.role == "last_frame" {
					var normalized map[string]any
					require.NoError(t, common.UnmarshalBodyReusable(c, &normalized))
					require.NotContains(t, normalized, "aspect_ratio")
					require.NotContains(t, normalized["parameters"], "aspect_ratio")
				}
				request.FillPromptFromContent()
				require.Equal(t, tc.model, request.Model)
				require.NotNil(t, request.Tool)
				require.Equal(t, expectedTool, *request.Tool)
				require.Equal(t, tc.duration, request.Duration)
				require.NotEmpty(t, request.Prompt)
				if tc.role != "" {
					require.Equal(t, tc.role, request.Content[0]["role"])
				}
				info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
				adaptor := &sora.TaskAdaptor{}
				adaptor.Init(info)
				require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
				require.Equal(t, float64(tc.duration), adaptor.EstimateBilling(c, info)["seconds"])
				upstreamBody, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				encoded, err := io.ReadAll(upstreamBody)
				require.NoError(t, err)
				var forwarded map[string]any
				require.NoError(t, common.Unmarshal(encoded, &forwarded))
				require.Equal(t, request.Model, forwarded["model"])
				require.Equal(t, expectedTool, forwarded["tool"])
				require.NotContains(t, forwarded, "mode", "native tools must retain their legacy capability semantics")
				require.Contains(t, forwarded, "resolution")
				require.NotContains(t, forwarded, "contents")
				c.Status(http.StatusOK)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		})
	}
}

func TestKlingNativeFrameCombinationsRejectedBeforeRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"contents":[{"type":"prompt","text":"test"},{"type":"first_frame","url":"https://example.com/start.png"},{"type":"refer_image","url":"https://example.com/ref.png"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"last_frame","url":"https://example.com/end.png"},{"type":"refer_image","url":"https://example.com/ref.png"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"first_frame","url":"https://example.com/start.png"},{"type":"feature_video","url":"https://example.com/ref.mp4"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"last_frame","url":"https://example.com/end.png"},{"type":"feature_video","url":"https://example.com/ref.mp4"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"first_frame","url":"https://example.com/start.png"}],"settings":{"aspect_ratio":"16:9"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			router := gin.New()
			called := false
			router.POST("/omni-video/kling-4.0", KlingNativeRequestConvert(), func(c *gin.Context) { called = true })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/omni-video/kling-4.0", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.False(t, called, "unsupported frame combination reached channel selection/billing")
		})
	}
}

func TestKlingNativeUnsupportedRejectedBeforeRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"prompt":"test","model":"kling-4.0","settings":{"resolution":"1080p"}}`,
		`{"prompt":"test","settings":{"duration":3}}`,
		`{"prompt":"test","settings":{"duration":4.5}}`,
		`{"prompt":"test","settings":{"duration":31}}`,
		`{"prompt":"test","settings":{"aspect_ratio":"adaptive"}}`,
		`{"prompt":"test","settings":{"seed":0}}`,
		`{"prompt":"test","settings":{"audio":false}}`,
		`{"prompt":"test","settings":{"resolution":"4k"}}`,
		`{"prompt":"test","settings":null}`,
		`{"prompt":"test","options":{"callback_url":"https://example.com/callback"}}`,
		`{"prompt":"test","options":{"external_task_id":"my-id"}}`,
		`{"prompt":"test","options":{"watermark_info":{"enabled":true}}}`,
		`{"prompt":"test","contents":[{"type":"voice","voice_id":"voice"}]}`,
		`{"prompt":"test","duration":4}`,
		`{`,
	} {
		t.Run(body, func(t *testing.T) {
			router := gin.New()
			called := false
			router.POST("/text-to-video/kling-4.0", KlingNativeRequestConvert(), func(c *gin.Context) { called = true })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/text-to-video/kling-4.0", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.False(t, called, "unsupported request reached channel selection/billing")
		})
	}
	for _, body := range []string{
		`{"contents":[{"type":"prompt","text":"test"},{"type":"refer_image","url":"https://example.com/x","id":"image1"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"middle_frame","url":"https://example.com/x"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"base_video","url":"https://example.com/x"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"element","element_id":"x"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"refer_image","url":"https://example.com/x"},{"type":"refer_image","url":"https://example.com/y"}]}`,
		`{"contents":[{"type":"prompt","text":"test"},{"type":"refer_image","url":"data:image/png;base64,AAAA"}]}`,
		`{"contents":[{"type":"prompt","text":"test"}]}`,
		`{"contents":[{"type":"refer_image","url":"https://example.com/x"}]}`,
	} {
		var request map[string]any
		require.NoError(t, common.Unmarshal([]byte(body), &request))
		_, err := normalizeKlingNativeRequest("/omni-video/kling-4.0", request)
		require.Error(t, err, body)
	}
}
