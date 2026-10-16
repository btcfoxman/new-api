package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/dto"
)

func TestLongChineseResponsesTaskPreviewIsValidUTF8(t *testing.T) {
	prompt := strings.Repeat("生成蓝色圆形", 3000)
	input, err := json.Marshal([]map[string]any{{
		"role":    "user",
		"content": []map[string]any{{"type": "input_text", "text": prompt}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := &dto.OpenAIResponsesRequest{Input: input}
	for label, preview := range map[string]string{
		"prompt": responsesPromptFromRequest(request),
		"input":  truncateResponsesTaskText(string(request.Input)),
	} {
		if !utf8.ValidString(preview) {
			t.Fatalf("%s preview is invalid UTF-8", label)
		}
		if len(preview) > 2000 {
			t.Fatalf("%s preview has %d bytes, want at most 2000", label, len(preview))
		}
		if preview == "" {
			t.Fatalf("%s preview is empty", label)
		}
	}
	if got := responsesPromptFromRequest(request); !strings.HasPrefix(prompt, got) {
		t.Fatalf("prompt preview changed the prompt prefix")
	}
}
