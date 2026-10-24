package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestViduQ4ArkBillingUsesThreeSecondsWithoutChangingOtherModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"vidu-q4", "vidu-q4-540p", "kling-4.0", "doubao-seedance-2-0"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
		c.Set("task_request", relaycommon.TaskSubmitReq{Model: name, Duration: 3})
		info := &relaycommon.RelayInfo{}
		enforceDoubaoOfficialDurationBilling(c, info)
		expected := float64(4)
		if relaycommon.IsViduQ4Model(name) {
			expected = 3
		}
		require.Equal(t, expected, info.PriceData.OtherRatios["seconds"])
	}
}

func TestViduQ4NativeQueryConvertsStoredOpenAIDataWithoutLeakingUpstreamID(t *testing.T) {
	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "vidu-q4"}, PrivateData: model.TaskPrivateData{ResultURL: "https://example.test/video.mp4"}}
	task.SetData(map[string]any{"id": "upstream-private", "status": "completed", "video_url": "https://example.test/video.mp4"})
	encoded := buildViduOfficialTaskResponse(task)
	var got map[string]any
	require.NoError(t, common.Unmarshal(encoded, &got))
	require.Equal(t, "task_public", got["task_id"])
	require.Equal(t, "success", got["state"])
	require.Len(t, got["creations"], 1)
	require.NotContains(t, string(encoded), "upstream-private")
	task.Properties.OriginModelName = "viduq2"
	legacy := buildViduOfficialTaskResponse(task)
	require.Contains(t, string(legacy), `"status":"completed"`)
}
