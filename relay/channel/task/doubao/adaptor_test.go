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
		Prompt: "draw this image",
		Content: []map[string]interface{}{
			{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": "https://example.com/input.png",
				},
			},
			{
				"type": "text",
				"text": "original text",
			},
		},
	}

	payload, err := adaptor.convertToRequestPayload(req)
	require.NoError(t, err)
	require.Len(t, payload.Content, 2)
	require.Equal(t, "image_url", payload.Content[0].Type)
	require.Equal(t, "https://example.com/input.png", payload.Content[0].ImageURL.URL)
	require.Equal(t, "text", payload.Content[1].Type)
	require.Equal(t, "draw this image", payload.Content[1].Text)
}
