package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestValidateBackgroundImageInputURLs(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Background: json.RawMessage(`true`),
		Tools:      json.RawMessage(`[{"type":"image_generation"}]`),
		Input:      json.RawMessage(`[{"role":"user","content":[{"type":"input_image","image_url":"https://example.test/input.jpg"}]}]`),
	}
	if err := ValidateBackgroundImageInputURLs(request); err != nil {
		t.Fatal(err)
	}
	request.Input = json.RawMessage(`[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]`)
	if err := ValidateBackgroundImageInputURLs(request); err == nil {
		t.Fatal("expected inline image to be rejected")
	}
}

func TestNormalizeImageResponsesURLBody(t *testing.T) {
	input := `{"id":"resp_1","status":"completed","output":[{"type":"image_generation_call","result":"` + strings.Repeat("A", 1024) + `","url":"https://example.test/output.png"}]}`
	output := NormalizeImageResponsesURLBody([]byte(input))
	if len(output) >= len(input) {
		t.Fatal("image bytes were not removed")
	}
	var document map[string]any
	if err := common.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	item := document["output"].([]any)[0].(map[string]any)
	if item["url"] != "https://example.test/output.png" || item["result"] != nil {
		t.Fatalf("unexpected image output: %#v", item)
	}
	input = `{"id":"resp_2","status":"completed","output":[{"type":"image_generation_call","result":"AAAA"}]}`
	output = NormalizeImageResponsesURLBody([]byte(input))
	if err := common.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document["status"] != "failed" || len(document["output"].([]any)) != 0 {
		t.Fatalf("missing URL should fail the task: %#v", document)
	}
	input = `{"id":"resp_3","status":"completed","output":[{"type":"image_generation","content":[{"type":"output_image","url":"https://example.test/third.png","b64_json":"AAAA"}]}]}`
	output = NormalizeImageResponsesURLBody([]byte(input))
	if err := common.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	item = document["output"].([]any)[0].(map[string]any)
	if item["type"] != "image_generation_call" || item["url"] != "https://example.test/third.png" {
		t.Fatalf("nested image URL was not normalized: %#v", item)
	}
	if _, found := item["content"]; found {
		t.Fatal("nested image bytes were retained")
	}
}
