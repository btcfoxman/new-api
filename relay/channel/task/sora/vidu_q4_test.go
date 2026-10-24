package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewaycommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestViduQ4OpenAIChannelForwardsThreeFormatsWithoutLosingAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/videos", "/api/v3/contents/generations/tasks", "/ent/v2/reference2video"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"vidu-q4","duration":3,"content":[{"type":"text","text":"A moving subject"},{"type":"image_url","image_url":{"url":"https://example.test/1.png"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://example.test/2.png"},"role":"reference_image"},{"type":"audio_url","audio_url":{"url":"https://example.test/1.mp3"},"role":"reference_audio"},{"type":"audio_url","audio_url":{"url":"https://example.test/2.mp3"},"role":"reference_audio"}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{OriginModelName: "vidu-q4", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "vidu-q4", ChannelBaseUrl: "https://api.example.test"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			a := &TaskAdaptor{}
			a.Init(info)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			encoded, err := io.ReadAll(body)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, gatewaycommon.Unmarshal(encoded, &payload))
			require.Equal(t, "vidu-q4", payload["model"])
			require.Equal(t, "720p", payload["resolution"])
			require.Equal(t, float64(3), payload["duration"])
			require.Equal(t, "reference_material", payload["mode"])
			require.Len(t, payload["images"], 2)
			require.Len(t, payload["audio_urls"], 2)
			require.Equal(t, float64(3), a.EstimateBilling(c, info)["seconds"])
		})
	}
}

func TestViduQ4NativeCreationOnOpenAIChannelReturnsPublicViduIDAndState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/ent/v2/reference2video", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "vidu-q4", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{"id":"upstream-private","status":"queued"}`))}
	id, _, err := (&TaskAdaptor{}).DoResponse(c, response, info)
	require.Nil(t, err)
	require.Equal(t, "upstream-private", id)
	var payload map[string]any
	require.NoError(t, gatewaycommon.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "task_public", payload["task_id"])
	require.Equal(t, "created", payload["state"])
	require.NotContains(t, w.Body.String(), "upstream-private")
}
