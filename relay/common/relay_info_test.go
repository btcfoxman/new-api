package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestValidateBasicTaskRequestTreatsImageLikeFieldsAsGenerate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "image",
			body: `{"prompt":"p","model":"m","image":"https://example.com/image.png"}`,
		},
		{
			name: "image_url",
			body: `{"prompt":"p","model":"m","image_url":"https://example.com/image-url.png"}`,
		},
		{
			name: "image_urls",
			body: `{"prompt":"p","model":"m","image_urls":["https://example.com/first.png"]}`,
		},
		{
			name: "images",
			body: `{"prompt":"p","model":"m","images":["https://example.com/images.png"]}`,
		},
		{
			name: "reference_images",
			body: `{"prompt":"p","model":"m","reference_images":[{"image_url":{"url":"https://example.com/reference.png"}}]}`,
		},
		{
			name: "input_reference",
			body: `{"prompt":"p","model":"m","input_reference":"https://example.com/input.png"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
			taskErr := ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
			require.Nil(t, taskErr)
			require.Equal(t, constant.TaskActionGenerate, info.Action)
		})
	}
}
