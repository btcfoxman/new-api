package controller

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type klingNativeTask struct {
	ID         string              `json:"id"`
	Status     string              `json:"status"`
	Message    string              `json:"message"`
	CreateTime int64               `json:"create_time"`
	UpdateTime int64               `json:"update_time"`
	ExternalID string              `json:"external_id"`
	Outputs    []klingNativeOutput `json:"outputs"`
}

type klingNativeOutput struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Duration string `json:"duration"`
}

func klingNativeEnvelope(c *gin.Context, status int, message string, data any) {
	code := 0
	if status >= http.StatusBadRequest {
		code = status
	}
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": c.GetString(common.RequestIdKey), "data": data})
}

// Only these non-streaming native routes buffer their relay response. This
// preserves the common relay's channel selection, billing and task persistence.
type klingNativeWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
	size   int
}

func (w *klingNativeWriter) WriteHeader(status int) { w.status = status }
func (w *klingNativeWriter) WriteHeaderNow() {
	if w.size < 0 {
		w.size = 0
	}
}
func (w *klingNativeWriter) Status() int   { return w.status }
func (w *klingNativeWriter) Size() int     { return w.size }
func (w *klingNativeWriter) Written() bool { return w.size >= 0 }
func (w *klingNativeWriter) Write(body []byte) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.Write(body)
	w.size += n
	return n, err
}
func (w *klingNativeWriter) WriteString(body string) (int, error) { return w.Write([]byte(body)) }

// KlingNativeResponse also wraps authentication and validation errors in the
// native envelope. Existing video and legacy Kling routes remain unchanged.
func KlingNativeResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		original := c.Writer
		buffer := &klingNativeWriter{ResponseWriter: original, status: http.StatusOK, size: -1}
		c.Writer = buffer
		defer func() { c.Writer = original }()
		c.Next()
		c.Writer = original
		c.Header("Content-Length", "")
		var body map[string]any
		_ = common.Unmarshal(buffer.body.Bytes(), &body)
		if buffer.status >= http.StatusBadRequest {
			message, _ := body["message"].(string)
			if detail, ok := body["error"].(map[string]any); ok {
				if text, ok := detail["message"].(string); ok && text != "" {
					message = text
				}
			}
			if message == "" {
				message = http.StatusText(buffer.status)
			}
			klingNativeEnvelope(c, buffer.status, message, nil)
			return
		}
		// The native query handler already returned a native array envelope.
		if code, ok := body["code"].(float64); ok && code == 0 && c.Request.Method == http.MethodGet && c.GetBool("kling_native_query_response") {
			c.Data(buffer.status, "application/json", buffer.body.Bytes())
			return
		}
		id, _ := body["id"].(string)
		if id == "" {
			id, _ = body["task_id"].(string)
		}
		if !strings.HasPrefix(id, "task_") {
			klingNativeEnvelope(c, http.StatusBadGateway, "invalid task response", nil)
			return
		}
		status, _ := body["status"].(string)
		created := klingNativeTimestamp(body["created_at"])
		if created == 0 {
			created = common.GetTimestamp() * 1000
		}
		updated := klingNativeTimestamp(body["updated_at"])
		if updated == 0 {
			updated = created
		}
		klingNativeEnvelope(c, buffer.status, "success", gin.H{
			"id": id, "status": klingNativeStatus(status), "create_time": created,
			"update_time": updated, "external_id": "",
		})
	}
}

func klingNativeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "processing", "in_progress", "running":
		return "processing"
	case "completed", "succeeded", "success":
		return "succeeded"
	case "failed", "failure", "error", "cancelled", "canceled":
		return "failed"
	default:
		return "submitted"
	}
}

func klingNativeTimestamp(value any) int64 {
	var result int64
	switch typed := value.(type) {
	case float64:
		result = int64(typed)
	case int64:
		result = typed
	case string:
		result, _ = strconv.ParseInt(typed, 10, 64)
	}
	if result > 0 && result < 100000000000 {
		result *= 1000
	}
	return result
}

// KlingNativeTaskFetch only reads tasks owned by the authenticated user and
// belonging to the two supported Kling 4.0 billing models. Polling stays in the
// existing task worker; no generation request is made by this endpoint.
func KlingNativeTaskFetch(c *gin.Context) {
	query := c.Request.URL.Query()
	if len(query) != 1 || len(query["task_ids"]) != 1 {
		klingNativeEnvelope(c, http.StatusBadRequest, "only task_ids query is supported", nil)
		return
	}
	ids := strings.Split(query.Get("task_ids"), ",")
	if len(ids) > 100 || len(ids) == 0 {
		klingNativeEnvelope(c, http.StatusBadRequest, "task_ids must contain 1 to 100 IDs", nil)
		return
	}
	tasks := make([]klingNativeTask, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			klingNativeEnvelope(c, http.StatusBadRequest, "task_ids must be nonempty and unique", nil)
			return
		}
		seen[id] = true
		task, exists, err := model.GetByTaskId(c.GetInt("id"), id)
		if err != nil {
			klingNativeEnvelope(c, http.StatusInternalServerError, "failed to read task", nil)
			return
		}
		if !exists || (task.Properties.OriginModelName != "kling-4.0" && task.Properties.OriginModelName != "kling-4.0-1080p") {
			klingNativeEnvelope(c, http.StatusNotFound, "task not found", nil)
			return
		}
		item := klingNativeTask{ID: task.TaskID, Status: klingNativeStatus(task.Status.ToVideoStatus()), CreateTime: klingNativeTimestamp(task.CreatedAt), UpdateTime: klingNativeTimestamp(task.UpdatedAt), Outputs: []klingNativeOutput{}}
		if task.Status == model.TaskStatusFailure {
			item.Message = task.FailReason
		}
		if task.Status == model.TaskStatusSuccess {
			if resultURL := task.GetResultURL(); resultURL != "" {
				item.Outputs = []klingNativeOutput{{Type: "video", ID: task.TaskID, URL: resultURL, Duration: klingNativeTaskDuration(task)}}
			}
		}
		tasks = append(tasks, item)
	}
	c.Set("kling_native_query_response", true)
	klingNativeEnvelope(c, http.StatusOK, "success", tasks)
}

func klingNativeTaskDuration(task *model.Task) string {
	for _, source := range [][]byte{task.Data, []byte(task.Properties.Input)} {
		var value map[string]any
		if common.Unmarshal(source, &value) != nil {
			continue
		}
		for _, key := range []string{"seconds", "duration"} {
			switch duration := value[key].(type) {
			case string:
				if _, err := strconv.ParseFloat(duration, 64); err == nil {
					return duration
				}
			case float64:
				return strconv.FormatFloat(duration, 'f', -1, 64)
			}
		}
	}
	return ""
}
