package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoCopiesPrompt(t *testing.T) {
	task := &model.Task{
		Prompt: "draw this image",
	}

	got := TaskModel2Dto(task)

	require.Equal(t, "draw this image", got.Prompt)
}

func TestNormalizeOfficialVideoBillingSecondsEnforcesMinimum(t *testing.T) {
	require.Equal(t, float64(4), normalizeOfficialVideoBillingSeconds(0, ""))
	require.Equal(t, float64(4), normalizeOfficialVideoBillingSeconds(1, ""))
	require.Equal(t, float64(4), normalizeOfficialVideoBillingSeconds(0, "3"))
	require.Equal(t, float64(8), normalizeOfficialVideoBillingSeconds(8, ""))
	require.Equal(t, float64(12), normalizeOfficialVideoBillingSeconds(0, "12"))
	require.Equal(t, float64(4), normalizeOfficialVideoBillingSeconds(0, "NaN"))
	require.Equal(t, float64(4), normalizeOfficialVideoBillingSeconds(0, "+Inf"))
}

func TestOfficialVideoEndpointSuppliesDurationWithoutForcingPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path            string
		expectsDuration bool
	}{
		{path: "/api/v3/contents/generations/tasks", expectsDuration: true},
		{path: "/oapi/v3/contents/generations/tasks", expectsDuration: true},
		{path: "/v1/videos", expectsDuration: false},
		{path: "/v1/video/generations", expectsDuration: false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{
				"model":"gemini-omni",
				"duration":10
			}`))
			c.Set("task_request", relaycommon.TaskSubmitReq{Model: "gemini-omni", Duration: 10})
			info := &relaycommon.RelayInfo{PriceData: types.PriceData{OtherRatios: map[string]float64{}}}

			enforceDoubaoOfficialDurationBilling(c, info)

			if tc.expectsDuration {
				require.Equal(t, float64(10), info.PriceData.OtherRatios["seconds"])
			} else {
				require.NotContains(t, info.PriceData.OtherRatios, "seconds")
			}
			require.False(t, c.GetBool("force_apply_task_other_ratios"))
		})
	}
}

func TestBuildTaskPricingInputNormalizesAllMatrixSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/kling/v1/videos/text2video", strings.NewReader(`{
		"model":"kling-v3",
		"prompt":"scene <<<voice_1>>>",
		"duration":10,
		"size":"1280x720",
		"mode":"pro",
		"metadata":{
			"sound":"on",
			"voice_list":["voice_1"]
		}
	}`))
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "kling-v3",
		Prompt:   "scene <<<voice_1>>>",
		Duration: 10,
		Size:     "1280x720",
		Mode:     "pro",
		Metadata: map[string]any{
			"sound":      "on",
			"voice_list": []any{"voice_1"},
		},
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "kling-v3",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kling-v3",
		},
	}

	input := buildTaskPricingInput(c, info, map[string]float64{"seconds": 10})

	require.Equal(t, "kling-v3", input.Model)
	require.Equal(t, float64(10), input.Duration)
	require.Equal(t, "720p", input.Resolution)
	require.Equal(t, "pro", input.Mode)
	require.True(t, input.Sound)
	require.True(t, input.Voice)
}

func TestBuildTaskPricingInputUsesKlingModelNameAndMetadataMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"kling-video",
		"model_name":"kling-v3",
		"mode":"t2v",
		"duration":10,
		"metadata":{"model_name":"kling-v3","mode":"pro","sound":"on"}
	}`))
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "kling-video",
		Mode:     "t2v",
		Duration: 10,
		Metadata: map[string]any{
			"model_name": "kling-v3",
			"mode":       "pro",
			"sound":      "on",
		},
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "kling-video",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kling-video",
		},
	}

	input := buildTaskPricingInput(c, info, nil)

	require.Equal(t, "kling-v3", input.Model)
	require.Equal(t, "pro", input.Mode)
	require.Equal(t, float64(10), input.Duration)
	require.True(t, input.Sound)
}

func TestBuildAliOfficialTaskResponseCopiesTopLevelVideoURLToOutput(t *testing.T) {
	task := &model.Task{
		TaskID: "task_123",
		Status: model.TaskStatusSuccess,
		Data: []byte(`{
			"id": "video_123",
			"object": "video",
			"status": "completed",
			"video_url": "https://example.com/video.mp4",
			"output": {
				"task_id": "upstream_task",
				"task_status": "SUCCEEDED"
			}
		}`),
	}

	body := buildAliOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Equal(t, "task_123", payload["request_id"])
	require.Equal(t, "task_123", payload["id"])
	require.Equal(t, "task_123", payload["task_id"])
	require.Equal(t, "video", payload["object"])
	require.Equal(t, "completed", payload["status"])
	require.Equal(t, "https://example.com/video.mp4", payload["video_url"])

	output, ok := payload["output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "task_123", output["task_id"])
	require.Equal(t, "SUCCEEDED", output["task_status"])
	require.Equal(t, "https://example.com/video.mp4", output["video_url"])
}

func TestBuildAliOfficialTaskResponseFallbackCopiesResultURLToOutput(t *testing.T) {
	task := &model.Task{
		TaskID: "task_123",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/fallback.mp4",
		},
	}

	body := buildAliOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	output, ok := payload["output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "task_123", output["task_id"])
	require.Equal(t, "SUCCEEDED", output["task_status"])
	require.Equal(t, "https://example.com/fallback.mp4", output["video_url"])
	require.Equal(t, "task_123", payload["request_id"])
}

func TestBuildAliOfficialTaskResponseUsesCurrentTaskStatus(t *testing.T) {
	task := &model.Task{
		TaskID: "task_123",
		Status: model.TaskStatusInProgress,
		Data: []byte(`{
			"id": "upstream_task",
			"status": "completed",
			"video_url": "https://example.com/stale.mp4",
			"request_id": "upstream_request",
			"output": {
				"task_id": "upstream_task",
				"task_status": "SUCCEEDED",
				"video_url": "https://example.com/stale.mp4"
			}
		}`),
	}

	body := buildAliOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Equal(t, "upstream_request", payload["request_id"])
	output, ok := payload["output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "task_123", output["task_id"])
	require.Equal(t, "RUNNING", output["task_status"])
	require.NotContains(t, output, "video_url")
	require.Equal(t, "task_123", payload["id"])
	require.Equal(t, "in_progress", payload["status"])
	require.NotContains(t, payload, "video_url")
}

func TestBuildAliOfficialTaskResponseFailureUsesCompatibleErrorFields(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		Status:     model.TaskStatusFailure,
		FailReason: "video generation was rejected",
	}

	body := buildAliOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Equal(t, "failed", payload["status"])
	errorData, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "generation_failed", errorData["code"])
	require.Equal(t, "video generation was rejected", errorData["message"])
	output, ok := payload["output"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "FAILED", output["task_status"])
	require.Equal(t, "generation_failed", output["code"])
	require.Equal(t, "video generation was rejected", output["message"])
	require.Equal(t, "task_failed", payload["request_id"])
}

func TestBuildDoubaoOfficialTaskResponseKeepLegacyAndOfficialFields(t *testing.T) {
	task := &model.Task{
		TaskID: "task_8ytcrEEGJa9NKz2g1zs0BayvWiZ9fdKA",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
		},
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		Data: []byte(`{
			"id":"video_bGl0ZWxsbTp...",
			"model":"doubao-seedance-2-0-260128-lyapi",
			"status":"completed",
			"video_url":"https://file-vercel-ly-no.aiid.edu.kg/cms_ai_video/3596008094/cgt-20260603141749-4jp4q.mp4",
			"usage":{"completion_tokens":108900,"total_tokens":108900},
			"created_at":1779348818,
			"updated_at":1779348874,
			"seed":78674,
			"resolution":"720p",
			"ratio":"16:9",
			"duration":5,
			"framespersecond":24,
			"service_tier":"default",
			"execution_expires_after":172800,
			"generate_audio":true,
			"draft":false,
			"priority":0,
			"tools":[{"type":"web_search"}],
			"error":null
		}`),
	}

	respBody := buildDoubaoOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(respBody, &payload))

	require.Equal(t, "success", payload["code"])
	require.Equal(t, "", payload["message"])
	require.NotNil(t, payload["data"])
	legacyData, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, task.TaskID, legacyData["task_id"])

	require.Equal(t, task.TaskID, payload["id"])
	require.Equal(t, "doubao-seedance-2-0-260128", payload["model"])
	require.Equal(t, "succeeded", payload["status"])
	content, ok := payload["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://file-vercel-ly-no.aiid.edu.kg/cms_ai_video/3596008094/cgt-20260603141749-4jp4q.mp4", content["video_url"])
	usage, ok := payload["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(108900), usage["completion_tokens"])
	require.Equal(t, float64(108900), usage["total_tokens"])
	require.Equal(t, float64(1779348818), payload["created_at"])
	require.Equal(t, float64(1779348874), payload["updated_at"])
	require.Equal(t, float64(78674), payload["seed"])
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, "16:9", payload["ratio"])
	require.Equal(t, float64(5), payload["duration"])
	require.Equal(t, float64(24), payload["framespersecond"])
	require.Equal(t, "default", payload["service_tier"])
	require.Equal(t, float64(172800), payload["execution_expires_after"])
	require.Equal(t, true, payload["generate_audio"])
	require.Equal(t, false, payload["draft"])
	require.Equal(t, float64(0), payload["priority"])
	tools, ok := payload["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	require.Nil(t, payload["error"])
}

func TestBuildDoubaoOfficialTaskResponseDefaultsMissingResolutionAndRatio(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_8ytcrEEGJa9NKz2g1zs0BayvWiZ9fdKA",
		CreatedAt: 1780467459,
		UpdatedAt: 1780467794,
		Status:    model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		Data: []byte(`{
			"id":"video_bGl0ZWxsbTp...",
			"model":"doubao-seedance-2-0-260128-lyapi",
			"object":"video",
			"status":"completed",
			"seconds":"4",
			"size":null,
			"usage":{"duration_seconds":4},
			"video_url":"https://file-vercel-ly-no.aiid.edu.kg/cms_ai_video/3596008094/cgt-20260603141749-4jp4q.mp4"
		}`),
	}

	respBody := buildDoubaoOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(respBody, &payload))

	require.Equal(t, task.TaskID, payload["id"])
	require.Equal(t, "doubao-seedance-2-0-260128", payload["model"])
	require.Equal(t, "succeeded", payload["status"])
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, "16:9", payload["ratio"])
	require.Equal(t, float64(4), payload["duration"])
	require.Equal(t, float64(1780467459), payload["created_at"])
	require.Equal(t, float64(1780467794), payload["updated_at"])
	require.Equal(t, float64(24), payload["framespersecond"])
	require.Equal(t, "default", payload["service_tier"])
	require.Equal(t, float64(172800), payload["execution_expires_after"])
	require.Equal(t, true, payload["generate_audio"])
	require.Equal(t, false, payload["draft"])
	require.Equal(t, float64(0), payload["priority"])

	content, ok := payload["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://file-vercel-ly-no.aiid.edu.kg/cms_ai_video/3596008094/cgt-20260603141749-4jp4q.mp4", content["video_url"])
	usage, ok := payload["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(4), usage["duration_seconds"])
	require.Equal(t, float64(0), usage["completion_tokens"])
	require.Equal(t, float64(0), usage["total_tokens"])
	require.NotNil(t, payload["data"])
}

func TestBuildDoubaoOfficialTaskResponseFailureIncludesOfficialErrorFields(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		CreatedAt:  1780467459,
		UpdatedAt:  1780467794,
		Status:     model.TaskStatusFailure,
		FailReason: "blocked by upstream",
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		Data: []byte(`{
			"id":"task_failed",
			"model":"doubao-seedance-2-0-260128-lyapi",
			"status":"failed",
			"error":{"code":"content_policy_violation","message":"policy blocked"},
			"tools":[{"type":"web_search"}]
		}`),
	}

	respBody := buildDoubaoOfficialTaskResponse(task)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(respBody, &payload))

	require.Equal(t, "failed", payload["status"])
	require.Equal(t, "success", payload["code"])
	require.Equal(t, "", payload["message"])
	require.NotNil(t, payload["data"])
	errorObj, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "content_policy_violation", errorObj["code"])
	require.Equal(t, "policy blocked", errorObj["message"])

	usage, ok := payload["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), usage["completion_tokens"])
	require.Equal(t, float64(0), usage["total_tokens"])

	tools, ok := payload["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
}

func TestBuildDoubaoPureOfficialTaskResponseOmitsLegacyFields(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_123",
		CreatedAt: 1780467459,
		UpdatedAt: 1780467794,
		Status:    model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
		},
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
	}

	respBody := buildDoubaoOfficialTaskResponseWithMode(task, true)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(respBody, &payload))

	require.Equal(t, "task_123", payload["id"])
	require.Equal(t, "doubao-seedance-2-0-260128", payload["model"])
	require.Equal(t, "succeeded", payload["status"])
	require.NotContains(t, payload, "code")
	require.NotContains(t, payload, "message")
	require.NotContains(t, payload, "data")
}
