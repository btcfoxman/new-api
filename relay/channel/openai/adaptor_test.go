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

func TestConvertImageRequestGenerationsPreservesFreebeatImageOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	request := dto.ImageRequest{
		Model:          "doubao-seedream-5-0-pro-260628",
		Prompt:         "change clothes",
		Image:          []byte(`"https://example.com/source.jpg"`),
		Ratio:          []byte(`"4:3"`),
		Resolution:     []byte(`"1k"`),
		Seed:           []byte(`123`),
		Count:          []byte(`1`),
		Names:          []byte(`["source"]`),
		ResponseFormat: "url",
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}

	got, err := (&Adaptor{}).ConvertImageRequest(ctx, info, request)
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, "doubao-seedream-5-0-pro-260628", gjson.GetBytes(encoded, "model").String())
	require.Equal(t, "change clothes", gjson.GetBytes(encoded, "prompt").String())
	require.Equal(t, "https://example.com/source.jpg", gjson.GetBytes(encoded, "image").String())
	require.Equal(t, "4:3", gjson.GetBytes(encoded, "ratio").String())
	require.Equal(t, "1k", gjson.GetBytes(encoded, "resolution").String())
	require.Equal(t, int64(123), gjson.GetBytes(encoded, "seed").Int())
	require.Equal(t, int64(1), gjson.GetBytes(encoded, "count").Int())
	require.Equal(t, "source", gjson.GetBytes(encoded, "names.0").String())
	require.Equal(t, "url", gjson.GetBytes(encoded, "response_format").String())
}
