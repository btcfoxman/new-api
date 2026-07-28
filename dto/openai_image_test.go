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

func TestImageRequestPreservesExtendedImageOptions(t *testing.T) {
	raw := []byte(`{
		"model": "doubao-seedream-5-0-pro-260628",
		"prompt": "change clothes",
		"image": "https://example.com/source.jpg",
		"ratio": "4:3",
		"aspect_ratio": "3:4",
		"aspectRatio": "16:9",
		"ar": "1:1",
		"aspect": "9:16",
		"resolution": "1k",
		"seed": 123,
		"count": 2,
		"names": ["source"],
		"image_names": ["source-a"],
		"imageNames": ["source-b"],
		"response_format": "url"
	}`)

	var req ImageRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.Equal(t, "4:3", gjson.GetBytes(encoded, "ratio").String())
	require.Equal(t, "3:4", gjson.GetBytes(encoded, "aspect_ratio").String())
	require.Equal(t, "16:9", gjson.GetBytes(encoded, "aspectRatio").String())
	require.Equal(t, "1:1", gjson.GetBytes(encoded, "ar").String())
	require.Equal(t, "9:16", gjson.GetBytes(encoded, "aspect").String())
	require.Equal(t, "1k", gjson.GetBytes(encoded, "resolution").String())
	require.Equal(t, int64(123), gjson.GetBytes(encoded, "seed").Int())
	require.Equal(t, int64(2), gjson.GetBytes(encoded, "count").Int())
	require.Equal(t, "source", gjson.GetBytes(encoded, "names.0").String())
	require.Equal(t, "source-a", gjson.GetBytes(encoded, "image_names.0").String())
	require.Equal(t, "source-b", gjson.GetBytes(encoded, "imageNames.0").String())
}
