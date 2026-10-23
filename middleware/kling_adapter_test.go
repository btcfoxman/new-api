package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKlingRequestConvertKeepsNativeAndLegacyRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		body   string
		native bool
	}{
		{"native", `{"model_name":"kling-4.0-1080p","contents":[{"type":"prompt","text":"A product shot"}],"settings":{"duration":4,"seed":0},"options":{"watermark":false}}`, true},
		{"legacy", `{"model_name":"kling-v2","prompt":"A product shot","duration":5}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/kling/v1/videos/text2video", KlingRequestConvert(), func(c *gin.Context) {
				require.Equal(t, "/v1/video/generations", c.Request.URL.Path)
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, int64(len(body)), c.Request.ContentLength)
				var converted map[string]interface{}
				require.NoError(t, common.Unmarshal(body, &converted))
				if tc.native {
					require.Equal(t, "kling-4.0-1080p", converted["model"])
					require.Len(t, converted["contents"], 1)
					require.Equal(t, false, converted["options"].(map[string]interface{})["watermark"])
					require.Equal(t, float64(0), converted["settings"].(map[string]interface{})["seed"])
				} else {
					require.Equal(t, "kling-v2", converted["model"])
					require.Equal(t, "A product shot", converted["prompt"])
					require.Equal(t, float64(5), converted["metadata"].(map[string]interface{})["duration"])
				}
				c.Status(http.StatusOK)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/kling/v1/videos/text2video", bytes.NewBufferString(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}
