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
			"resolution":"720p",
			"ratio":"16:9",
			"duration":5
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
	require.Equal(t, "720p", payload["resolution"])
	require.Equal(t, "16:9", payload["ratio"])
	require.Equal(t, float64(5), payload["duration"])
}

func TestBuildDoubaoOfficialTaskResponseDefaultsMissingResolutionAndRatio(t *testing.T) {
	task := &model.Task{
		TaskID: "task_8ytcrEEGJa9NKz2g1zs0BayvWiZ9fdKA",
		Status: model.TaskStatusSuccess,
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

	content, ok := payload["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://file-vercel-ly-no.aiid.edu.kg/cms_ai_video/3596008094/cgt-20260603141749-4jp4q.mp4", content["video_url"])
	require.NotNil(t, payload["data"])
}
