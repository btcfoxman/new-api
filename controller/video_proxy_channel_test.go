package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func strictVideoFetchPolicy(t *testing.T) {
	t.Helper()
	setting := system_setting.GetFetchSetting()
	before := *setting
	*setting = system_setting.FetchSetting{
		EnableSSRFProtection: true, AllowedPorts: []string{"80", "443"}, ApplyIPFilterForDomain: true,
	}
	t.Cleanup(func() { *setting = before })
}

func TestChannelVideoContentURLKeepsConfiguredOriginAndOpaqueID(t *testing.T) {
	for _, base := range []string{"http://litellm:4000", "http://192.168.3.5:4000/gateway/", "https://api.example.com"} {
		id := "video_ab+/=:_-"
		target, err := channelVideoContentURL(base, id)
		require.NoError(t, err)
		u, err := url.Parse(target)
		require.NoError(t, err)
		configured, err := url.Parse(base)
		require.NoError(t, err)
		require.Equal(t, configured.Scheme, u.Scheme)
		require.Equal(t, configured.Host, u.Host)
		require.Equal(t, strings.TrimRight(configured.Path, "/")+"/v1/videos/"+url.PathEscape(id)+"/content", u.EscapedPath())
		require.Empty(t, u.RawQuery)
		require.Empty(t, u.Fragment)
	}
}

func TestChannelVideoContentURLRejectsAmbiguousBasesAndIDInjection(t *testing.T) {
	for _, base := range []string{"", "//litellm:4000", "file:///tmp/video", "http://key@litellm:4000", "http://litellm:4000?url=other", "http://litellm:4000#other", "http://litellm:4000?"} {
		_, err := channelVideoContentURL(base, "video_123")
		require.Error(t, err, base)
	}
	for _, id := range []string{"", ".", "..", "../admin", "video_123/../../admin", "video_123?x=1", "video_123#x", "%2e%2e%2fadmin", "video_123\\admin", "https://evil.example", "video_123\r\nX-Test: yes"} {
		_, err := channelVideoContentURL("http://litellm:4000", id)
		require.Error(t, err, id)
	}
}

func TestChannelVideoContentRedirectKeepsSSRFAndStripsCredentials(t *testing.T) {
	strictVideoFetchPolicy(t)
	called := false
	original := &http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		called = true
		require.Empty(t, req.Header.Get("Authorization"))
		return nil
	}}
	client := channelVideoContentClient(original)
	require.NotSame(t, original, client)
	for _, target := range []string{"http://127.0.0.1/video.mp4", "http://8.8.8.8:4000/video.mp4", "http://169.254.169.254/latest/meta-data"} {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer private-test-key")
		require.Error(t, client.CheckRedirect(req, nil), target)
		require.Empty(t, req.Header.Get("Authorization"))
	}
	require.False(t, called)
	req, err := http.NewRequest(http.MethodGet, "https://8.8.8.8/video.mp4", nil)
	require.NoError(t, err)
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-Api-Key", "X-Goog-Api-Key"} {
		req.Header.Set(name, "private-test-value")
	}
	require.NoError(t, client.CheckRedirect(req, nil))
	require.True(t, called)
	for name := range req.Header {
		require.Empty(t, req.Header.Get(name))
	}
	require.Error(t, client.CheckRedirect(req, make([]*http.Request, 10)))
}

func videoContentTestDB(t *testing.T) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemoryCache := common.MemoryCacheEnabled
	setupTokenControllerTestDB(t)
	common.MemoryCacheEnabled = false
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Channel{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled = oldMemoryCache
	})
}

func requestVideoContent(userID int, id string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+id+"/content", nil)
	c.Params = gin.Params{{Key: "task_id", Value: id}}
	c.Set("id", userID)
	VideoProxy(c)
	return w
}

func TestVideoProxyConfiguredChannelStreamsOwnedCompletedTask(t *testing.T) {
	strictVideoFetchPolicy(t)
	videoContentTestDB(t)
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/videos/video_stored_123/content", r.URL.Path)
		require.Equal(t, "Bearer test-channel-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "test-video-bytes")
	}))
	defer upstream.Close()
	base := upstream.URL
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeSora} {
		channel := model.Channel{Type: channelType, Key: "test-channel-key", BaseURL: &base}
		require.NoError(t, model.DB.Create(&channel).Error)
		task := model.Task{TaskID: fmt.Sprintf("public_task_%d", channelType), UserId: 41, ChannelId: channel.Id,
			Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "video_stored_123", ResultURL: "http://169.254.169.254/ignored"}}
		require.NoError(t, model.DB.Create(&task).Error)
		// Ownership checks happen before the configured first-hop exception.
		otherUser := requestVideoContent(42, task.TaskID)
		require.Equal(t, http.StatusNotFound, otherUser.Code)
		before := requests
		response := requestVideoContent(41, task.TaskID)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, "video/mp4", response.Header().Get("Content-Type"))
		require.Equal(t, "test-video-bytes", response.Body.String())
		require.Equal(t, before+1, requests)
		require.NoError(t, model.DB.Model(&task).Update("status", model.TaskStatusInProgress).Error)
		require.Equal(t, http.StatusBadRequest, requestVideoContent(41, task.TaskID).Code)
		require.Equal(t, before+1, requests)
	}
}

func TestVideoProxyUntrustedResultAndRedirectCannotUseChannelException(t *testing.T) {
	strictVideoFetchPolicy(t)
	videoContentTestDB(t)
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("private redirect target must never be requested")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer redirected.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirected.URL+"/private.mp4", http.StatusFound)
	}))
	defer upstream.Close()
	base := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "test-channel-key", BaseURL: &base}
	require.NoError(t, model.DB.Create(&channel).Error)
	task := model.Task{TaskID: "public_redirect", UserId: 41, ChannelId: channel.Id,
		Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "video_123"}}
	require.NoError(t, model.DB.Create(&task).Error)
	require.Equal(t, http.StatusBadGateway, requestVideoContent(41, task.TaskID).Code)
	// A non-Sora provider's arbitrary URL is still subject to the original policy.
	require.NoError(t, model.DB.Model(&channel).Update("type", constant.ChannelTypeVidu).Error)
	task.PrivateData.ResultURL = "http://127.0.0.1:4000/private.mp4"
	require.NoError(t, model.DB.Model(&task).Update("private_data", task.PrivateData).Error)
	require.Equal(t, http.StatusForbidden, requestVideoContent(41, task.TaskID).Code)
}
