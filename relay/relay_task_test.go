package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	require.NotContains(t, payload, "id")
	require.NotContains(t, payload, "object")
	require.NotContains(t, payload, "status")
	require.NotContains(t, payload, "video_url")

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
