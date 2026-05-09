package openai

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertImageRequestEditsJSONPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	request := dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "replace the background",
		Images:         []byte(`[{"image_url":"https://example.com/source.png"}]`),
		ResponseFormat: "b64_json",
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}

	got, err := (&Adaptor{}).ConvertImageRequest(ctx, info, request)
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", gjson.GetBytes(encoded, "model").String())
	require.Equal(t, "replace the background", gjson.GetBytes(encoded, "prompt").String())
	require.Equal(t, "https://example.com/source.png", gjson.GetBytes(encoded, "images.0.image_url").String())
	require.Equal(t, "b64_json", gjson.GetBytes(encoded, "response_format").String())
}
