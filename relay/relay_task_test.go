package relay

import (
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
