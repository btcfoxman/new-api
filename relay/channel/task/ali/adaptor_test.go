package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHappyHorseOfficialRequestBodyKeepsDashScopeShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{
		"model": "happyhorse-1.0-t2v",
		"mode": "t2v",
		"input": {
			"prompt": "A white horse running through a neon city street, cinematic",
			"media": [{"type": "first_frame", "url": "https://example.com"}]
		},
		"parameters": {
			"resolution": "720P",
			"ratio": "16:9",
			"duration": 5,
			"watermark": true,
			"seed": 1
		}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "happyhorse-1.0-t2v",
		},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "happyhorse-1.0-t2v", payload["model"])
	require.NotContains(t, payload, "mode")

	input, ok := payload["input"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "A white horse running through a neon city street, cinematic", input["prompt"])
	media, ok := input["media"].([]any)
	require.True(t, ok)
	require.Len(t, media, 1)

	parameters, ok := payload["parameters"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "720P", parameters["resolution"])
	require.Equal(t, "16:9", parameters["ratio"])
	require.Equal(t, float64(5), parameters["duration"])
}

func TestHappyHorseAggregateModelResolvesByMode(t *testing.T) {
	require.Equal(t, "happyhorse-1.0-t2v", mustResolveHappyHorseModel(t, "happyhorse-1.0", "t2v"))
	require.Equal(t, "happyhorse-1.0-i2v", mustResolveHappyHorseModel(t, "happyhorse-1.0", "i2v"))
	require.Equal(t, "happyhorse-1.0-r2v", mustResolveHappyHorseModel(t, "happyhorse-1.0", "r2v"))
	require.Equal(t, "happyhorse-1.0-video-edit", mustResolveHappyHorseModel(t, "happyhorse-1.0", "video_edit"))
}

func mustResolveHappyHorseModel(t *testing.T, model string, mode string) string {
	t.Helper()
	resolved, err := resolveHappyHorseProviderModel(model, mode)
	require.NoError(t, err)
	return resolved
}
