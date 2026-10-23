package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKlingNativeSoraAdaptorSubmitResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/text-to-video/kling-4.0", KlingNativeResponse(), middleware.KlingNativeRequestConvert(), func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "kling-4.0"}}
		adaptor := &sora.TaskAdaptor{}
		adaptor.Init(info)
		require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
		require.Equal(t, float64(6), adaptor.EstimateBilling(c, info)["seconds"])
		requestBody, err := adaptor.BuildRequestBody(c, info)
		require.NoError(t, err)
		encoded, err := io.ReadAll(requestBody)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"model":"kling-4.0"`)
		require.Contains(t, string(encoded), `"resolution":"720p"`)
		upstream := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"gen_private_upstream","task_id":"gen_private_upstream","status":"queued","created_at":1700000000,"seconds":"6","metadata":{"api_key":"private-secret"}}`))}
		upstreamID, _, taskErr := adaptor.DoResponse(c, upstream, info)
		require.Nil(t, taskErr)
		require.Equal(t, "gen_private_upstream", upstreamID)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/text-to-video/kling-4.0", strings.NewReader(`{"prompt":"A cinematic camera move","settings":{"resolution":"720p","duration":6}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Zero(t, response.Code)
	require.Equal(t, "task_public", response.Data["id"])
	require.Equal(t, "submitted", response.Data["status"])
	require.Len(t, response.Data, 5)
	require.NotContains(t, recorder.Body.String(), "private")
}

func TestKlingNativeResponseEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		status int
		body   gin.H
	}{
		{"submitted", 200, gin.H{"id": "task_public", "task_id": "task_public", "status": "queued", "created_at": 1700000000, "metadata": gin.H{"private": "never expose"}}},
		{"processing", 200, gin.H{"id": "task_public", "status": "in_progress"}},
		{"invalid upstream id", 200, gin.H{"id": "provider-secret-id"}},
		{"submit code zero", 200, gin.H{"code": 0, "id": "task_public", "status": "queued", "private": "never expose", "data": gin.H{"key": "never expose"}}},
		{"submit code zero invalid shape", 200, gin.H{"code": 0, "data": gin.H{"private": "never expose"}}},
		{"validation", 400, gin.H{"message": "unsupported contents.type: voice"}},
		{"authentication", 401, gin.H{"error": gin.H{"message": "invalid token"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/text-to-video/kling-4.0", KlingNativeResponse(), func(c *gin.Context) {
				c.Set(common.RequestIdKey, "request_public")
				c.JSON(tc.status, tc.body)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/text-to-video/kling-4.0", nil))
			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "request_public", response["request_id"])
			require.NotContains(t, recorder.Body.String(), "never expose")
			require.NotContains(t, recorder.Body.String(), "provider-secret-id")
			if tc.status >= 400 || tc.name == "invalid upstream id" || tc.name == "submit code zero invalid shape" {
				require.GreaterOrEqual(t, recorder.Code, 400)
				require.NotEqual(t, float64(0), response["code"])
				require.Nil(t, response["data"])
			} else {
				require.Equal(t, 200, recorder.Code)
				require.Equal(t, float64(0), response["code"])
				data := response["data"].(map[string]any)
				require.Equal(t, "task_public", data["id"])
				require.Equal(t, "", data["external_id"])
				require.Contains(t, data, "create_time")
				if tc.name == "submitted" {
					require.Equal(t, "submitted", data["status"])
					require.Equal(t, float64(1700000000000), data["create_time"])
				} else if tc.name == "processing" {
					require.Equal(t, "processing", data["status"])
				}
			}
		})
	}
}

func TestKlingNativeQueryOwnershipAndOutput(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	for _, task := range []*model.Task{
		{TaskID: "task_done", UserId: 7, Status: model.TaskStatusSuccess, CreatedAt: 1700000000, UpdatedAt: 1700000010, Properties: model.Properties{OriginModelName: "kling-4.0-1080p", Input: `{"duration":10}`}, PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/result.mp4", UpstreamTaskID: "hidden-upstream-id", Key: "hidden-key"}},
		{TaskID: "task_pending", UserId: 7, Status: model.TaskStatusQueued, Properties: model.Properties{OriginModelName: "kling-4.0"}},
		{TaskID: "task_failed", UserId: 7, Status: model.TaskStatusFailure, FailReason: "specific normalized failure", Properties: model.Properties{OriginModelName: "kling-4.0"}},
		{TaskID: "task_other_user", UserId: 8, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "kling-4.0"}},
		{TaskID: "task_other_model", UserId: 7, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "wan-2.6"}},
	} {
		require.NoError(t, db.Create(task).Error)
	}
	router := gin.New()
	router.GET("/tasks", KlingNativeResponse(), func(c *gin.Context) { c.Set("id", 7) }, KlingNativeTaskFetch)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"task_ids=task_done,task_pending,task_failed", 200},
		{"task_ids=task_other_user", 404},
		{"task_ids=task_other_model", 404},
		{"task_ids=task_missing", 404},
		{"task_ids=task_done,task_other_user", 404},
		{"external_task_ids=mine", 400},
		{"task_ids=task_done&external_task_ids=mine", 400},
		{"task_ids=task_done&task_ids=task_pending", 400},
		{"task_ids=task_done,task_done", 400},
		{"task_ids=", 400},
		{"task_ids=" + strings.Repeat("task_done,", 100) + "task_pending", 400},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/tasks?"+tc.query, nil))
		require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "hidden-")
		if tc.status == 200 {
			var response struct {
				Code int               `json:"code"`
				Data []klingNativeTask `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Zero(t, response.Code)
			require.Len(t, response.Data, 3)
			require.Equal(t, "succeeded", response.Data[0].Status)
			require.Equal(t, int64(1700000000000), response.Data[0].CreateTime)
			require.Equal(t, "https://example.com/result.mp4", response.Data[0].Outputs[0].URL)
			require.Equal(t, "10", response.Data[0].Outputs[0].Duration)
			require.Equal(t, "submitted", response.Data[1].Status)
			require.Equal(t, "specific normalized failure", response.Data[2].Message)
		} else {
			require.NotContains(t, recorder.Body.String(), "result.mp4")
		}
	}
}
