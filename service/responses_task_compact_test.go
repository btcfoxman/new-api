package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestCompactImageResponsesTaskBodyDropsImageBytes(t *testing.T) {
	raw := `{"id":"resp_1","status":"completed","output":[{"type":"image_generation_call","result":"` + strings.Repeat("A", 1024) + `","b64_json":"secret","url":"https://example.test/result.png"}]}`
	compact := compactImageResponsesTaskBody([]byte(raw))
	if len(compact) >= len(raw) {
		t.Fatalf("task body was not compacted")
	}
	var document map[string]any
	if err := common.Unmarshal(compact, &document); err != nil {
		t.Fatal(err)
	}
	item := document["output"].([]any)[0].(map[string]any)
	if item["result"] != nil || item["url"] != "https://example.test/result.png" {
		t.Fatalf("unexpected task output: %#v", item)
	}
	if _, found := item["b64_json"]; found {
		t.Fatalf("Base64 was persisted")
	}
}
