package sora

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func publicVideoResultPolicy(t *testing.T) {
	t.Helper()
	setting := system_setting.GetFetchSetting()
	before := *setting
	*setting = system_setting.FetchSetting{EnableSSRFProtection: true, AllowedPorts: []string{"80", "443"}, ApplyIPFilterForDomain: true}
	t.Cleanup(func() { *setting = before })
}

func TestCompletedPublicVideoResultPreservesURLAndRejectsPrivateOrMalformed(t *testing.T) {
	publicVideoResultPolicy(t)
	public := "https://8.8.8.8/results/video.mp4?version=1"
	require.Equal(t, public, completedPublicVideoURL("completed", public))
	for _, status := range []string{"queued", "in_progress", "failed"} {
		require.Empty(t, completedPublicVideoURL(status, public))
	}
	for _, value := range []string{"", "http://127.0.0.1/video.mp4", "http://169.254.169.254/latest/meta-data", "http://192.168.3.5/video.mp4", "http://8.8.8.8:4000/video.mp4", "https://key@8.8.8.8/video.mp4", "file:///tmp/video.mp4", "data:video/mp4;base64,Zml4dHVyZQ==", "//8.8.8.8/video.mp4"} {
		require.Empty(t, completedPublicVideoURL("completed", value), value)
	}
	// Allowing private authenticated downloads must not publish private URLs.
	setting := system_setting.GetFetchSetting()
	setting.EnableSSRFProtection = false
	setting.AllowPrivateIp = true
	setting.ApplyIPFilterForDomain = false
	setting.AllowedPorts = []string{"80", "443", "4000"}
	require.Empty(t, completedPublicVideoURL("completed", "http://127.0.0.1:4000/v1/assets/private"))
}

func TestSoraCompletedTaskParsesPublicVideoURLWithoutInventingContentLinks(t *testing.T) {
	publicVideoResultPolicy(t)
	for _, body := range []string{
		`{"id":"upstream_123","status":"completed","video_url":"https://8.8.8.8/results/s3.mp4"}`,
		`{"id":"upstream_123","status":"completed","output":{"video_url":"https://8.8.8.8/results/s3.mp4"}}`,
	} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
		require.NoError(t, err)
		require.Equal(t, model.TaskStatusSuccess, result.Status)
		require.Equal(t, "https://8.8.8.8/results/s3.mp4", result.Url)
	}
	for _, body := range []string{
		`{"status":"completed"}`,
		`{"status":"completed","video_url":"http://127.0.0.1:4000/v1/assets/private"}`,
		`{"status":"failed","video_url":"https://8.8.8.8/results/s3.mp4","error":{"code":"E","message":"preserved reason"}}`,
	} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
		require.NoError(t, err)
		require.Empty(t, result.Url)
		if result.Status == model.TaskStatusFailure {
			require.Equal(t, "preserved reason", result.Reason)
		}
	}
}

func TestSoraCreateResponseExposesOnlyCompletedPublicResult(t *testing.T) {
	publicVideoResultPolicy(t)
	gin.SetMode(gin.TestMode)
	for _, test := range []struct{ status, url, expected string }{
		{"completed", "https://8.8.8.8/results/s3.mp4", "https://8.8.8.8/results/s3.mp4"},
		{"completed", "http://127.0.0.1:4000/v1/assets/private", ""},
		{"queued", "https://8.8.8.8/results/s3.mp4", ""},
	} {
		body, err := json.Marshal(map[string]any{"id": "upstream_123", "status": test.status, "video_url": test.url})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public_123"}}
		id, _, taskErr := (&TaskAdaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, info)
		require.Nil(t, taskErr)
		require.Equal(t, "upstream_123", id)
		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, "public_123", response["id"])
		if test.expected == "" {
			require.NotContains(t, response, "video_url")
		} else {
			require.Equal(t, test.expected, response["video_url"])
		}
	}
}

func TestSoraOpenAIProjectionUsesSafeStoredResultOrOwnedContent(t *testing.T) {
	publicVideoResultPolicy(t)
	before := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = before })
	for _, test := range []struct{ url, expected string }{
		{"https://8.8.8.8/results/s3.mp4", "https://8.8.8.8/results/s3.mp4"},
		{"http://127.0.0.1:4000/v1/assets/private", "https://gateway.example/v1/videos/public_123/content"},
		{"", "https://gateway.example/v1/videos/public_123/content"},
	} {
		task := &model.Task{TaskID: "public_123", Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{ResultURL: test.url},
			Data:        json.RawMessage(`{"id":"upstream_private","status":"completed","video_url":"http://127.0.0.1/private"}`)}
		data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
		require.NoError(t, err)
		var response map[string]any
		require.NoError(t, json.Unmarshal(data, &response))
		require.Equal(t, "public_123", response["id"])
		require.Equal(t, test.expected, response["video_url"])
		require.NotContains(t, string(data), "127.0.0.1")
		task.Status = model.TaskStatusInProgress
		data, err = (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &response))
		require.NotContains(t, string(data), "video_url")
	}
}
