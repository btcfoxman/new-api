package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadKeepsTopLevelContentMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model:  "doubao-video",
		Mode:   "i2v_first_last",
		Prompt: "draw this image",
		Content: []map[string]interface{}{
			{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": "https://example.com/input.png",
				},
				"role": "first_frame",
				"name": "image1",
			},
			{
				"type": "text",
				"text": "original text",
			},
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	require.Equal(t, "i2v_first_last", payload.Mode)
	require.Len(t, payload.Content, 2)
	require.Equal(t, "image_url", payload.Content[0].Type)
	require.Equal(t, "https://example.com/input.png", payload.Content[0].ImageURL.URL)
	require.Equal(t, "first_frame", payload.Content[0].Role)
	require.Equal(t, "image1", payload.Content[0].Name)
	require.Equal(t, "text", payload.Content[1].Type)
	require.Equal(t, "draw this image", payload.Content[1].Text)
}

func TestConvertKlingFourOfficialRequest(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := &relaycommon.TaskSubmitReq{
		Model: "kling-4.0", Prompt: "A product shot", Duration: 4,
		Content: []map[string]interface{}{
			{"type": "image_url", "image_url": "https://example.com/image.jpg"},
			{"type": "video_url", "video_url": "https://example.com/video.mp4"},
		},
		Metadata: map[string]interface{}{"duration": 4, "resolution": "720p", "ratio": "16:9"},
		Options:  map[string]interface{}{"watermark": false},
	}
	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	require.Equal(t, "kling-4.0", payload.Model)
	require.Equal(t, "720p", payload.Resolution)
	require.Equal(t, "16:9", payload.Ratio)
	require.Equal(t, map[string]interface{}{"watermark": false}, payload.Options)
	require.Len(t, payload.Content, 3)
	require.Equal(t, "image_url", payload.Content[0].Type)
	require.Equal(t, "video_url", payload.Content[1].Type)
}

func TestNormalizeDoubaoBillingSecondsEnforcesMinimum(t *testing.T) {
	require.Equal(t, float64(4), normalizeDoubaoBillingSeconds(0, ""))
	require.Equal(t, float64(4), normalizeDoubaoBillingSeconds(1, ""))
	require.Equal(t, float64(4), normalizeDoubaoBillingSeconds(3, ""))
	require.Equal(t, float64(4), normalizeDoubaoBillingSeconds(4, ""))
	require.Equal(t, float64(8), normalizeDoubaoBillingSeconds(8, ""))
	require.Equal(t, float64(8), normalizeDoubaoBillingSeconds(0, "8"))
	require.Equal(t, float64(4), normalizeDoubaoBillingSeconds(0, "1"))
}
