package kling

import "testing"

func TestEstimateKlingUnitDeduction(t *testing.T) {
	tests := []struct {
		name     string
		payload  *requestPayload
		metadata map[string]any
		want     float64
	}{
		{
			name:     "v1_6_pro_5s",
			payload:  &requestPayload{ModelName: "kling-v1-6", Mode: "pro", Duration: "5"},
			metadata: map[string]any{},
			want:     3.5,
		},
		{
			name:     "v2_5_turbo_std_5s",
			payload:  &requestPayload{ModelName: "kling-v2-5-turbo", Mode: "std", Duration: "5"},
			metadata: map[string]any{},
			want:     1.5,
		},
		{
			name:     "v2_6_pro_sound_voice",
			payload:  &requestPayload{ModelName: "kling-v2-6", Mode: "pro", Duration: "5", Prompt: "男人<<<voice_1>>>说：你好"},
			metadata: map[string]any{"sound": "on", "voice_list": []any{map[string]any{"voice_id": "1"}}},
			want:     6,
		},
		{
			name:     "v3_std_sound",
			payload:  &requestPayload{ModelName: "kling-v3", Mode: "std", Duration: "10"},
			metadata: map[string]any{"sound": "on"},
			want:     9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateKlingUnitDeduction(tt.payload, tt.metadata)
			if got != tt.want {
				t.Fatalf("estimateKlingUnitDeduction() = %v, want %v", got, tt.want)
			}
		})
	}
}
