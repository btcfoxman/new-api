package service

import (
	"errors"
	"strings"
	"testing"
)

func TestTaskErrorWrapperSanitizesBase64FileNameTooLong(t *testing.T) {
	payload := strings.Repeat("A", 500)
	taskErr := TaskErrorWrapper(
		errors.New("[Errno 36] File name too long: 'data:image/png;base64,"+payload+"'"),
		"test_error",
		500,
	)
	if strings.Contains(taskErr.Message, payload) {
		t.Fatalf("message contains full base64 payload: %s", taskErr.Message)
	}
	if strings.Contains(taskErr.Error.Error(), payload) {
		t.Fatalf("error contains full base64 payload: %s", taskErr.Error.Error())
	}
	if !strings.Contains(taskErr.Message, "...[truncated") {
		t.Fatalf("message missing truncation marker: %s", taskErr.Message)
	}
}

func TestTaskErrorWrapperSanitizesRawBase64WhenFilenameTooLong(t *testing.T) {
	payload := strings.Repeat("B", 500)
	taskErr := TaskErrorWrapper(
		errors.New("[Errno 36] File name too long: '"+payload+"'"),
		"test_error",
		500,
	)
	if strings.Contains(taskErr.Message, payload) {
		t.Fatalf("message contains full raw base64 payload: %s", taskErr.Message)
	}
	if !strings.Contains(taskErr.Message, "...[truncated") {
		t.Fatalf("message missing truncation marker: %s", taskErr.Message)
	}
}
