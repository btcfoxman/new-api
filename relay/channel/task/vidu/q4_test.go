package vidu

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestViduQ4NativePayloadKeepsAllMediaAndSeparateAudioToggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/ent/v2/reference2video", strings.NewReader(`{"model":"vidu-q4-540p","prompt":"A moving subject","duration":3,"images":["https://example.test/1.png","https://example.test/2.png"],"audio_urls":["https://example.test/1.mp3","https://example.test/2.mp3"],"audio":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: "vidu-q4-540p", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVidu, UpstreamModelName: "vidu-q4-540p", ChannelBaseUrl: "https://api.example.test"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	require.Equal(t, constant.TaskActionReferenceGenerate, info.Action)
	url, err := a.BuildRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.example.test/ent/v2/reference2video", url)
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload requestPayload
	require.NoError(t, common.Unmarshal(encoded, &payload))
	require.Equal(t, "vidu-q4-540p", payload.Model)
	require.Equal(t, "540p", payload.Resolution)
	require.Equal(t, 3, payload.Duration)
	require.Len(t, payload.Images, 2)
	require.Len(t, payload.AudioURLs, 2)
	require.True(t, payload.Audio)
	require.Equal(t, map[string]float64{"seconds": 3}, a.EstimateBilling(c, info))
	model, group := viduBillingRuleTarget(info)
	require.Equal(t, "vidu-q4-540p", model)
	require.Equal(t, "", group)
	require.Contains(t, a.GetModelList(), "viduq2")
	require.Contains(t, a.GetModelList(), "vidu2.0")
}
