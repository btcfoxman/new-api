package suno

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestActionValidateDefaultsMusicToSunoV55(t *testing.T) {
	request := &dto.SunoSubmitReq{Model: "suno_music"}

	err := actionValidate(nil, request, constant.SunoActionMusic)

	require.NoError(t, err)
	require.Equal(t, "suno-v5.5", request.UpstreamModel)
	require.Equal(t, "suno-v5.5", resolveSunoBillingModel(request))
}

func TestActionValidateKeepsExplicitLegacySunoV5(t *testing.T) {
	request := &dto.SunoSubmitReq{
		Model:         "suno_music",
		UpstreamModel: "suno-v5",
	}

	err := actionValidate(nil, request, constant.SunoActionMusic)

	require.NoError(t, err)
	require.Equal(t, "suno-v5", request.UpstreamModel)
	require.Equal(t, "suno-v5", resolveSunoBillingModel(request))
}

func TestActionValidateBillsExplicitSunoV6(t *testing.T) {
	request := &dto.SunoSubmitReq{
		Model:         "suno_music",
		UpstreamModel: "suno-v6",
	}

	err := actionValidate(nil, request, constant.SunoActionMusic)

	require.NoError(t, err)
	require.Equal(t, "suno-v6", request.UpstreamModel)
	require.Equal(t, "suno-v6", resolveSunoBillingModel(request))
}

func TestNormalizeSunoMusicBillingModelSupportsFreebeatAliases(t *testing.T) {
	cases := map[string]string{
		"suno-v5.5":             "suno-v5.5",
		"chirp-fenix":           "suno-v5.5",
		"128":                   "suno-v5.5",
		"seed-audio-1.0":        "seed-audio-1.0",
		"126":                   "seed-audio-1.0",
		"minimax-speech-2.8-hd": "minimax-speech-2.8-hd",
		"speech-2.8-hd":         "minimax-speech-2.8-hd",
		"127":                   "minimax-speech-2.8-hd",
		"lyria-3-pro":           "lyria-3-pro",
		"lyria-3-pro-preview":   "lyria-3-pro",
		"129":                   "lyria-3-pro",
	}
	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeSunoMusicBillingModel(input))
		})
	}
	require.Equal(t, "suno-v5.5", normalizeSunoMusicBillingModel(128))
	require.Equal(t, "seed-audio-1.0", normalizeSunoMusicBillingModel(126))
}

func TestSunoSubmitRequestPreservesSpeechParameters(t *testing.T) {
	sampleRate := 44100
	speed := 1.1
	volume := 0.8
	pitch := 0.0
	request := &dto.SunoSubmitReq{
		Model:         "suno_music",
		UpstreamModel: "minimax-speech-2.8-hd",
		ExtraParams: map[string]interface{}{
			"voice": "custom_voice",
		},
		Voice:      "top_level_voice",
		SampleRate: &sampleRate,
		Speed:      &speed,
		Volume:     &volume,
		Pitch:      &pitch,
	}

	data, err := common.Marshal(request)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &payload))

	require.NotContains(t, payload, "credits")
	require.Equal(t, "custom_voice", payload["extraParams"].(map[string]interface{})["voice"])
	require.Equal(t, "top_level_voice", payload["voice"])
	require.Equal(t, float64(44100), payload["sample_rate"])
	require.Equal(t, float64(1.1), payload["speed"])
	require.Equal(t, float64(0.8), payload["volume"])
	require.Contains(t, payload, "pitch")
	require.Equal(t, float64(0), payload["pitch"])
}

func TestResolveSunoDynamicCredits(t *testing.T) {
	seedPrompt := &dto.SunoSubmitReq{Prompt: "1234567890123456"}
	require.Equal(t, 4, resolveSunoDynamicCredits(seedPrompt, "seed-audio-1.0"))

	speechPrompt := &dto.SunoSubmitReq{Prompt: "语音"}
	require.Equal(t, 10, resolveSunoDynamicCredits(speechPrompt, "minimax-speech-2.8-hd"))

	longSpeechPrompt := &dto.SunoSubmitReq{Prompt: strings.Repeat("a", 200)}
	require.Equal(t, 20, resolveSunoDynamicCredits(longSpeechPrompt, "minimax-speech-2.8-hd"))

	require.Equal(t, 0, resolveSunoDynamicCredits(seedPrompt, "suno-v5.5"))
}

func TestSunoSubmitRequestDropsClientCredits(t *testing.T) {
	var request dto.SunoSubmitReq
	require.NoError(t, common.Unmarshal(
		[]byte(`{"model":"suno_music","upstream_model":"seed-audio-1.0","prompt":"sound","credits":1}`),
		&request,
	))

	data, err := common.Marshal(request)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &payload))

	require.NotContains(t, payload, "credits")
	require.Equal(t, 2, resolveSunoDynamicCredits(&request, "seed-audio-1.0"))
}
