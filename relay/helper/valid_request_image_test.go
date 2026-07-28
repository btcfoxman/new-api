package helper

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGetAndValidOpenAIImageRequestPreservesExtendedJSONOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{
		"model": "doubao-seedream-5-0-pro-260628",
		"prompt": "change clothes",
		"image": "https://example.com/source.jpg",
		"ratio": "4:3",
		"resolution": "1k",
		"seed": 123,
		"count": 1,
		"names": ["source"]
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	req, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/source.jpg", gjson.GetBytes(encoded, "image").String())
	require.Equal(t, "4:3", gjson.GetBytes(encoded, "ratio").String())
	require.Equal(t, "1k", gjson.GetBytes(encoded, "resolution").String())
	require.Equal(t, int64(123), gjson.GetBytes(encoded, "seed").Int())
	require.Equal(t, int64(1), gjson.GetBytes(encoded, "count").Int())
	require.Equal(t, "source", gjson.GetBytes(encoded, "names.0").String())
}

func TestGetAndValidOpenAIImageRequestPreservesExtendedMultipartOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "doubao-seedream-5-0-pro-260628"))
	require.NoError(t, writer.WriteField("prompt", "change clothes"))
	require.NoError(t, writer.WriteField("image", "https://example.com/source.jpg"))
	require.NoError(t, writer.WriteField("ratio", "4:3"))
	require.NoError(t, writer.WriteField("resolution", "1k"))
	require.NoError(t, writer.WriteField("seed", "123"))
	require.NoError(t, writer.WriteField("count", "1"))
	require.NoError(t, writer.WriteField("names", "source"))
	require.NoError(t, writer.Close())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	req, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/source.jpg", gjson.GetBytes(encoded, "image").String())
	require.Equal(t, "4:3", gjson.GetBytes(encoded, "ratio").String())
	require.Equal(t, "1k", gjson.GetBytes(encoded, "resolution").String())
	require.Equal(t, "123", gjson.GetBytes(encoded, "seed").String())
	require.Equal(t, "1", gjson.GetBytes(encoded, "count").String())
	require.Equal(t, "source", gjson.GetBytes(encoded, "names").String())
}
