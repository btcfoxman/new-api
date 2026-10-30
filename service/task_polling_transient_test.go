package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

// This adaptor has no submission or network path. Tests exercise the actual
// polling/CAS/billing code against the package's disposable SQLite fixture.
type transientPollingAdaptor struct {
	status      int
	body        string
	fetchErr    error
	parseErr    error
	readErr     bool
	result      relaycommon.TaskInfo
	fetchedIDs  []string
	parseCalls  int
	adjustCalls int
}

func (*transientPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *transientPollingAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	a.fetchedIDs = append(a.fetchedIDs, body["task_id"].(string))
	if a.fetchErr != nil {
		return nil, a.fetchErr
	}
	response := &http.Response{StatusCode: a.status, Body: io.NopCloser(strings.NewReader(a.body))}
	if a.readErr {
		response.Body = failingPollingBody{}
	}
	return response, nil
}
func (a *transientPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	a.parseCalls++
	if a.parseErr != nil {
		return nil, a.parseErr
	}
	result := a.result
	return &result, nil
}
func (a *transientPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	a.adjustCalls++
	return 0
}

type failingPollingBody struct{}

func (failingPollingBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingPollingBody) Close() error             { return nil }

func seedTransientPollingTask(t *testing.T) (*model.Task, *model.Channel) {
	t.Helper()
	truncate(t)
	seedUser(t, 910, 900)
	seedToken(t, 910, 910, "synthetic-polling-key", 900)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 910).Update("used_quota", 100).Error)
	seedChannel(t, 910)
	task := makeTask(910, 910, 100, 910, BillingSourceWallet, 0)
	task.TaskID = fmt.Sprintf("task_poll_%d", time.Now().UnixNano())
	task.PrivateData.UpstreamTaskID = "original-accepted-upstream-id"
	task.PrivateData.Key = "original-synthetic-upstream-key"
	task.SubmitTime = time.Now().Unix() - 30
	task.StartTime = task.SubmitTime + 1
	task.Progress = "37%"
	task.Data = []byte(`{"id":"original-accepted-upstream-id","status":"in_progress","progress":37}`)
	require.NoError(t, model.DB.Create(task).Error)
	return reloadPollingTask(t, task.ID), &model.Channel{Id: 910, Key: "synthetic-channel-key"}
}

func reloadPollingTask(t *testing.T, id int64) *model.Task {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.First(&task, id).Error)
	return &task
}

func pollFixtureTask(task *model.Task, channel *model.Channel, adaptor *transientPollingAdaptor) error {
	id := task.GetUpstreamTaskID()
	return updateVideoSingleTask(context.Background(), adaptor, channel, id, map[string]*model.Task{id: task})
}

func requirePollingTaskUnchanged(t *testing.T, before, inMemory *model.Task) {
	t.Helper()
	require.Equal(t, before, inMemory, "failed lookup must not mutate the last valid in-memory snapshot")
	require.Equal(t, before, reloadPollingTask(t, before.ID), "failed lookup must not persist a terminal state or replace task data")
	require.Equal(t, 900, getUserQuota(t, 910))
	require.Equal(t, 900, getTokenRemainQuota(t, 910))
	require.Equal(t, 100, getTokenUsedQuota(t, 910))
	require.Zero(t, countLogs(t))
}

func TestVideoPollingTransientHTTPDoesNotMutateOrRefund(t *testing.T) {
	statuses := []int{408, 425, 429, 500, 502, 503, 504, 520, 521, 522, 523, 524, 525, 526, 527}
	for _, status := range statuses {
		for _, body := range []string{
			`{"error":{"code":"500","message":"video status is temporarily unavailable; retry polling."}}`,
			`<html><title>Cloudflare gateway unavailable</title></html>`,
		} {
			t.Run(fmt.Sprintf("HTTP%d_HTML%t", status, strings.HasPrefix(body, "<")), func(t *testing.T) {
				task, channel := seedTransientPollingTask(t)
				before := reloadPollingTask(t, task.ID)
				// Even a misleading body/adaptor failure must not turn an HTTP
				// lookup failure into an authoritative task failure.
				adaptor := &transientPollingAdaptor{status: status, body: body,
					result: relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "not a task response"}}
				require.Error(t, pollFixtureTask(task, channel, adaptor))
				require.Zero(t, adaptor.parseCalls)
				require.Zero(t, adaptor.adjustCalls)
				requirePollingTaskUnchanged(t, before, task)
			})
		}
	}
}

func TestVideoPollingJSONWithoutTaskStatusDoesNotFailTask(t *testing.T) {
	for _, body := range []string{
		`{"error":{"code":"500","message":"api[l] - api[l] video status is temporarily unavailable; retry polling.","param":null,"type":null}}`,
		`{"error":{"code":500,"message":"temporary query failure"}}`,
		`{"error":{"code":"429","message":"rate limited"}}`,
		`{"error":{"code":429,"message":"rate limited"}}`,
		`{"error":"temporary query failure"}`,
		`{}`,
		`null`,
		`{"message":"video status is temporarily unavailable; retry polling."}`,
		`{"success":false,"msg":"temporary gateway failure"}`,
	} {
		t.Run(fmt.Sprintf("body%d", len(body)), func(t *testing.T) {
			task, channel := seedTransientPollingTask(t)
			before := reloadPollingTask(t, task.ID)
			adaptor := &transientPollingAdaptor{status: http.StatusOK, body: body}
			require.Error(t, pollFixtureTask(task, channel, adaptor))
			require.Equal(t, 1, adaptor.parseCalls)
			requirePollingTaskUnchanged(t, before, task)
		})
	}
}

func TestVideoPollingTransportAndMalformedRepliesKeepOriginalTask(t *testing.T) {
	for _, adaptor := range []*transientPollingAdaptor{
		{fetchErr: context.DeadlineExceeded},
		{status: http.StatusOK, readErr: true},
		{status: http.StatusOK, body: "<html>temporary edge failure</html>", parseErr: fmt.Errorf("invalid JSON")},
	} {
		t.Run(fmt.Sprintf("transport%t_read%t", adaptor.fetchErr != nil, adaptor.readErr), func(t *testing.T) {
			task, channel := seedTransientPollingTask(t)
			before := reloadPollingTask(t, task.ID)
			require.Error(t, pollFixtureTask(task, channel, adaptor))
			requirePollingTaskUnchanged(t, before, task)
		})
	}
}

func TestVideoPollingResumesOriginalIDFromDatabaseAfterTransientQueries(t *testing.T) {
	task, channel := seedTransientPollingTask(t)
	originalSubmitTime := task.SubmitTime
	adaptor := &transientPollingAdaptor{status: http.StatusServiceUnavailable, body: `{"error":{"message":"retry polling"}}`}
	require.Error(t, pollFixtureTask(task, channel, adaptor))
	var resumed *model.Task
	for _, pending := range model.GetAllUnFinishSyncTasks(100) {
		if pending.ID == task.ID {
			resumed = pending
		}
	}
	require.NotNil(t, resumed, "a new process must still select the original accepted task")
	require.Equal(t, originalSubmitTime, resumed.SubmitTime)
	require.Equal(t, "original-accepted-upstream-id", resumed.GetUpstreamTaskID())
	adaptor.status = http.StatusOK
	adaptor.body = `{"id":"original-accepted-upstream-id","status":"completed"}`
	adaptor.result = relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: "https://example.invalid/original-result.mp4"}
	require.NoError(t, pollFixtureTask(resumed, channel, adaptor))
	completed := reloadPollingTask(t, task.ID)
	require.EqualValues(t, model.TaskStatusSuccess, completed.Status)
	require.Equal(t, task.TaskID, completed.TaskID)
	require.Equal(t, task.PrivateData.UpstreamTaskID, completed.PrivateData.UpstreamTaskID)
	require.Equal(t, originalSubmitTime, completed.SubmitTime)
	require.False(t, completed.PrivateData.Refunded)
	require.Equal(t, []string{"original-accepted-upstream-id", "original-accepted-upstream-id"}, adaptor.fetchedIDs)
	require.Equal(t, 1, adaptor.adjustCalls)
	require.Equal(t, 900, getUserQuota(t, 910))
	require.Equal(t, 900, getTokenRemainQuota(t, 910))
	require.Equal(t, 100, getTokenUsedQuota(t, 910))
	for _, pending := range model.GetAllUnFinishSyncTasks(100) {
		require.NotEqual(t, task.ID, pending.ID, "completed task must not be polled again")
	}
}

func TestVideoPollingAuthoritativeFailureStillRefundsOnlyCASWinner(t *testing.T) {
	task, channel := seedTransientPollingTask(t)
	stale := reloadPollingTask(t, task.ID)
	adaptor := &transientPollingAdaptor{status: http.StatusOK,
		body:   `{"id":"original-accepted-upstream-id","status":"failed","error":{"code":"CONTENT_POLICY","message":"actual task failure"}}`,
		result: relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "actual task failure"}}
	require.NoError(t, pollFixtureTask(task, channel, adaptor))
	failed := reloadPollingTask(t, task.ID)
	require.EqualValues(t, model.TaskStatusFailure, failed.Status)
	require.Equal(t, "actual task failure", failed.FailReason)
	require.True(t, failed.PrivateData.Refunded)
	require.Equal(t, 1000, getUserQuota(t, 910))
	require.Equal(t, 1000, getTokenRemainQuota(t, 910))
	require.Zero(t, getTokenUsedQuota(t, 910))
	require.EqualValues(t, 1, countLogs(t))
	// A different poller holding the pre-failure snapshot loses the existing CAS.
	require.NoError(t, pollFixtureTask(stale, channel, adaptor))
	require.Equal(t, 1000, getUserQuota(t, 910))
	require.Equal(t, 1000, getTokenRemainQuota(t, 910))
	require.Zero(t, getTokenUsedQuota(t, 910))
	require.EqualValues(t, 1, countLogs(t))
}

func TestVideoPollingTransientDoesNotResetAbsoluteTaskTimeout(t *testing.T) {
	task, channel := seedTransientPollingTask(t)
	previousTimeout := constant.TaskTimeoutMinutes
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	constant.TaskTimeoutMinutes = 1
	task.SubmitTime = time.Now().Unix() - 120
	require.NoError(t, model.DB.Save(task).Error)
	before := reloadPollingTask(t, task.ID)
	adaptor := &transientPollingAdaptor{status: http.StatusGatewayTimeout, body: "temporary lookup outage"}
	require.Error(t, pollFixtureTask(task, channel, adaptor))
	requirePollingTaskUnchanged(t, before, task)
	sweepTimedOutTasks(context.Background())
	failed := reloadPollingTask(t, task.ID)
	require.EqualValues(t, model.TaskStatusFailure, failed.Status)
	require.Equal(t, before.SubmitTime, failed.SubmitTime)
	require.Equal(t, before.PrivateData.UpstreamTaskID, failed.PrivateData.UpstreamTaskID)
	require.True(t, failed.PrivateData.Refunded)
}
