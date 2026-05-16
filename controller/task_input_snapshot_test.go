package controller

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTaskInputSnapshotFromMapKeepsOnlyImageFields(t *testing.T) {
	snapshot := buildTaskInputSnapshotFromMap(map[string]any{
		"model":         "sora-2",
		"prompt":        "draw",
		"image_url":     "https://example.com/input.png",
		"end_image_url": "https://example.com/end.png",
		"size":          "720x1280",
	})
	if snapshot == "" {
		t.Fatal("snapshot is empty")
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(snapshot), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got["image_url"] != "https://example.com/input.png" {
		t.Fatalf("image_url = %v", got["image_url"])
	}
	if got["end_image_url"] != "https://example.com/end.png" {
		t.Fatalf("end_image_url = %v", got["end_image_url"])
	}
	if _, ok := got["model"]; ok {
		t.Fatal("model should not be included")
	}
	if _, ok := got["prompt"]; ok {
		t.Fatal("prompt should not be included")
	}
	if _, ok := got["size"]; ok {
		t.Fatal("size should not be included")
	}
}

func TestBuildTaskInputSnapshotFromMapSkipsEmptyValues(t *testing.T) {
	snapshot := buildTaskInputSnapshotFromMap(map[string]any{
		"image":            "",
		"image_urls":       []any{},
		"reference_images": []any{map[string]any{"image_url": ""}},
	})
	if snapshot != "" {
		t.Fatalf("snapshot = %q, want empty", snapshot)
	}
}

func TestBuildTaskInputSnapshotFromMapTruncatesBase64Values(t *testing.T) {
	payload := strings.Repeat("A", 400)
	snapshot := buildTaskInputSnapshotFromMap(map[string]any{
		"image": "data:image/png;base64," + payload,
	})
	if strings.Contains(snapshot, payload) {
		t.Fatal("snapshot contains full base64 payload")
	}
	if !strings.Contains(snapshot, "...[truncated 392 chars]") {
		t.Fatalf("snapshot missing truncation marker: %s", snapshot)
	}
}

func TestBuildTaskInputSnapshotFromMapTruncatesNestedBase64ImageFields(t *testing.T) {
	payload := strings.Repeat("B", 360)
	rawPayload := strings.Repeat("C", 320)
	snapshot := buildTaskInputSnapshotFromMap(map[string]any{
		"image_url": map[string]any{
			"url": "data:image/jpeg;base64," + payload,
		},
		"image_references": []any{
			map[string]any{"image_url": rawPayload},
		},
	})
	if snapshot == "" {
		t.Fatal("snapshot is empty")
	}
	if strings.Contains(snapshot, payload) {
		t.Fatal("snapshot contains full nested data URI payload")
	}
	if strings.Contains(snapshot, rawPayload) {
		t.Fatal("snapshot contains full nested raw base64 payload")
	}
	if !strings.Contains(snapshot, "...[truncated 353 chars]") {
		t.Fatalf("snapshot missing data URI truncation marker: %s", snapshot)
	}
	if !strings.Contains(snapshot, "...[truncated 290 chars]") {
		t.Fatalf("snapshot missing raw base64 truncation marker: %s", snapshot)
	}
}

func TestBuildTaskInputSnapshotFromMapKeepsLongNonBase64ImageURL(t *testing.T) {
	longURL := "https://example.com/image.png?token=" + strings.Repeat("A", 320)
	snapshot := buildTaskInputSnapshotFromMap(map[string]any{
		"image_url": longURL,
	})
	if !strings.Contains(snapshot, longURL) {
		t.Fatalf("snapshot should keep normal image url: %s", snapshot)
	}
}
