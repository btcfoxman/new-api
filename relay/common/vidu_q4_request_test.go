package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestViduQ4ReferencesAreMergedWithoutLosingAudioOrMutatingInput(t *testing.T) {
	raw := map[string]any{"model": "vidu-q4", "prompt": "A moving subject", "duration": 3,
		"images":     []string{"https://example.test/1.png", "https://example.test/2.png"},
		"audio_urls": []string{"https://example.test/1.mp3", "https://example.test/2.mp3"},
		"content":    []map[string]any{{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/1.png"}}},
		"metadata":   map[string]any{"images": []string{"https://example.test/2.png"}, "model_name": "other-model"}}
	before, _ := json.Marshal(raw)
	got, action, err := NormalizeViduQ4Request(raw, "vidu-q4", "/ent/v2/reference2video")
	require.NoError(t, err)
	require.Equal(t, constant.TaskActionReferenceGenerate, action)
	require.Equal(t, []string{"https://example.test/1.png", "https://example.test/2.png"}, got["images"])
	require.Equal(t, []string{"https://example.test/1.mp3", "https://example.test/2.mp3"}, got["audio_urls"])
	require.Equal(t, "reference_material", got["mode"])
	require.Equal(t, 3, got["duration"])
	require.Equal(t, "720p", got["resolution"])
	require.NotContains(t, got["metadata"], "model_name")
	after, _ := json.Marshal(raw)
	require.Equal(t, string(before), string(after))
}

func TestViduQ4ModelDimensionsAndMediaGuards(t *testing.T) {
	for _, tc := range []struct {
		name        string
		change      map[string]any
		model, path string
		valid       bool
	}{
		{"minimum", map[string]any{"duration": 3}, "vidu-q4", "/v1/videos", true},
		{"maximum", map[string]any{"duration": 16}, "vidu-q4", "/api/v3/contents/generations/tasks", true},
		{"540p default", map[string]any{}, "vidu-q4-540p", "/ent/v2/img2video", true},
		{"reference mode", map[string]any{"mode": "reference_images"}, "vidu-q4", "/v1/videos", true},
		{"no images", map[string]any{"images": []string{}}, "vidu-q4", "/v1/videos", false},
		{"no audio only", map[string]any{"images": []string{}, "audio_urls": []string{"https://example.test/audio.mp3"}}, "vidu-q4", "/v1/videos", false},
		{"wrong resolution", map[string]any{"resolution": "1080p"}, "vidu-q4", "/v1/videos", false},
		{"wrong fixed suffix", map[string]any{"resolution": "720p"}, "vidu-q4-540p", "/v1/videos", false},
		{"too short", map[string]any{"duration": 2}, "vidu-q4", "/v1/videos", false},
		{"too long", map[string]any{"duration": 17}, "vidu-q4", "/v1/videos", false},
		{"fractional", map[string]any{"duration": 3.5}, "vidu-q4", "/v1/videos", false},
		{"conflicting seconds", map[string]any{"duration": 3, "seconds": "4"}, "vidu-q4", "/v1/videos", false},
		{"video rejected", map[string]any{"video_urls": []string{"https://example.test/v.mp4"}}, "vidu-q4", "/v1/videos", false},
		{"native text rejected", map[string]any{}, "vidu-q4", "/ent/v2/text2video", false},
		{"native firstlast rejected", map[string]any{}, "vidu-q4", "/ent/v2/start-end2video", false},
		{"explicit firstlast rejected", map[string]any{"mode": "i2v_first_last"}, "vidu-q4", "/v1/videos", false},
		{"last frame rejected", map[string]any{"last_image_url": "https://example.test/end.png"}, "vidu-q4", "/v1/videos", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"model": tc.model, "prompt": "test", "images": []string{"https://example.test/image.png"}}
			for k, v := range tc.change {
				raw[k] = v
			}
			got, _, err := NormalizeViduQ4Request(raw, tc.model, tc.path)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, tc.model, got["model"])
			} else {
				require.Error(t, err)
			}
		})
	}
	for _, n := range []int{15, 16} {
		images := []string{}
		for i := 0; i < n; i++ {
			images = append(images, fmt.Sprintf("https://example.test/%d.png", i))
		}
		_, _, err := NormalizeViduQ4Request(map[string]any{"images": images}, "vidu-q4", "/v1/videos")
		require.Equal(t, n == 15, err == nil)
	}
	for _, n := range []int{3, 4} {
		audio := []string{}
		for i := 0; i < n; i++ {
			audio = append(audio, fmt.Sprintf("https://example.test/%d.mp3", i))
		}
		_, _, err := NormalizeViduQ4Request(map[string]any{"images": []string{"https://example.test/1.png"}, "audio_urls": audio}, "vidu-q4", "/v1/videos")
		require.Equal(t, n == 3, err == nil)
	}
}

func TestViduQ4SingleAndMultipleImageRoutingKeepsLegacyModels(t *testing.T) {
	for _, n := range []int{1, 2, 3} {
		images := []string{}
		for i := 0; i < n; i++ {
			images = append(images, fmt.Sprintf("https://example.test/%d.png", i))
		}
		got, action, err := NormalizeViduQ4Request(map[string]any{"images": images, "mode": "i2v"}, "vidu-q4", "/ent/v2/img2video")
		require.NoError(t, err)
		if n == 1 {
			require.Equal(t, constant.TaskActionGenerate, action)
			require.Equal(t, "i2v", got["mode"])
		} else {
			require.Equal(t, constant.TaskActionReferenceGenerate, action)
			require.Equal(t, "reference_images", got["mode"])
		}
		require.Len(t, got["images"], n)
	}
	for _, model := range []string{"viduq2", "vidu2.0", "viduq3-turbo", "kling-4.0"} {
		raw := map[string]any{"mode": "i2v_first_last", "resolution": "1080p"}
		got, action, err := NormalizeViduQ4Request(raw, model, "/ent/v2/start-end2video")
		require.NoError(t, err)
		require.Empty(t, action)
		require.True(t, reflect.DeepEqual(raw, got))
	}
}

func TestViduQ4StringMetadataCannotOverrideValidatedDimensions(t *testing.T) {
	for _, metadata := range []string{`{"duration":16}`, `{"resolution":"540p"}`, `{"model_name":"other-model","duration":16}`} {
		_, _, err := NormalizeViduQ4Request(map[string]any{"duration": 3, "images": []string{"https://example.test/1.png"}, "metadata": metadata}, "vidu-q4", "/v1/videos")
		require.Error(t, err)
	}
	got, _, err := NormalizeViduQ4Request(map[string]any{"images": []string{"https://example.test/1.png"}, "metadata": `{"duration":5,"resolution":"720p","style":"cinematic"}`}, "vidu-q4", "/v1/videos")
	require.NoError(t, err)
	require.Equal(t, 5, got["duration"])
	require.Equal(t, map[string]any{"style": "cinematic"}, got["metadata"])
}
