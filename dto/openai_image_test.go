package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageRequestPreservesReferenceImageFields(t *testing.T) {
	raw := []byte(`{
		"model": "gpt-image-2",
		"prompt": "ad image",
		"image_url": "https://example.com/reference.png",
		"image_urls": ["https://example.com/reference-2.png"],
		"reference_images": [{"image_url": {"url": "https://example.com/reference-3.png"}}],
		"input_reference": {"url": "https://example.com/reference-4.png"},
		"urls": ["https://example.com/reference-5.png"],
		"response_format": "url"
	}`)

	var req ImageRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.Equal(t, "https://example.com/reference.png", gjson.GetBytes(encoded, "image_url").String())
	require.Equal(t, "https://example.com/reference-2.png", gjson.GetBytes(encoded, "image_urls.0").String())
	require.Equal(t, "https://example.com/reference-3.png", gjson.GetBytes(encoded, "reference_images.0.image_url.url").String())
	require.Equal(t, "https://example.com/reference-4.png", gjson.GetBytes(encoded, "input_reference.url").String())
	require.Equal(t, "https://example.com/reference-5.png", gjson.GetBytes(encoded, "urls.0").String())
}
