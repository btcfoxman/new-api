package controller

import (
	"encoding/json"
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
