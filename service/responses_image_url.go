package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func IsLikelyImageResponsesTask(modelName string, responseBody []byte) bool {
	if bytes.Contains(responseBody, []byte(`"image_generation_call"`)) {
		return true
	}
	modelName = strings.ToLower(modelName)
	return strings.Contains(modelName, "image") || strings.Contains(modelName, "seedream") ||
		strings.Contains(modelName, "flux") || strings.Contains(modelName, "banana")
}

func ValidateBackgroundImageInputURLs(request *dto.OpenAIResponsesRequest) error {
	if request == nil || !IsResponsesBackgroundRequest(request) || !IsResponsesImageGenerationRequest(request) {
		return nil
	}
	var input any
	if err := common.Unmarshal(request.Input, &input); err != nil {
		return fmt.Errorf("invalid background image input: %w", err)
	}
	var visit func(any) error
	visit = func(value any) error {
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				if err := visit(child); err != nil {
					return err
				}
			}
		case map[string]any:
			if item["type"] == "input_image" {
				url, ok := item["image_url"].(string)
				if !ok || (!strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://")) {
					return fmt.Errorf("background image input_image.image_url must be an HTTP(S) URL")
				}
			}
			for _, child := range item {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(input)
}

// NormalizeImageResponsesURLBody is an output guard for every Responses image
// adapter. It prevents an upstream regression from returning image Base64 to
// the caller or persisting it in task.Data.
func NormalizeImageResponsesURLBody(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"image_generation"`)) &&
		!bytes.Contains(body, []byte(`"image_generation_call"`)) {
		return body
	}
	var document map[string]json.RawMessage
	if err := common.Unmarshal(body, &document); err != nil {
		return body
	}
	var outputs []map[string]json.RawMessage
	if err := common.Unmarshal(document["output"], &outputs); err != nil {
		return body
	}
	changed := false
	missingURL := false
	for _, output := range outputs {
		var outputType string
		_ = common.Unmarshal(output["type"], &outputType)
		if outputType != dto.ResponsesOutputTypeImageGenerationCall && outputType != "image_generation" {
			continue
		}
		changed = true
		output["type"] = json.RawMessage(`"image_generation_call"`)
		var url string
		_ = common.Unmarshal(output["url"], &url)
		if url == "" {
			_ = common.Unmarshal(output["result"], &url)
		}
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			var content []map[string]json.RawMessage
			if err := common.Unmarshal(output["content"], &content); err == nil {
				for _, item := range content {
					_ = common.Unmarshal(item["url"], &url)
					if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
						break
					}
					url = ""
				}
			}
		}
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			missingURL = true
			break
		}
		encodedURL, err := common.Marshal(url)
		if err != nil {
			missingURL = true
			break
		}
		output["url"] = encodedURL
		output["result"] = json.RawMessage("null")
		delete(output, "b64_json")
		delete(output, "image")
		delete(output, "content")
	}
	if !changed {
		return body
	}
	if missingURL {
		// A generated image with no URL cannot be delivered on this contract.
		// Report it as a terminal task failure without submitting again.
		document["status"] = json.RawMessage(`"failed"`)
		document["output"] = json.RawMessage(`[]`)
		document["error"] = json.RawMessage(`{"code":"missing_image_url","message":"Upstream image result has no downloadable URL"}`)
	} else {
		encoded, err := common.Marshal(outputs)
		if err != nil {
			return body
		}
		document["output"] = encoded
	}
	encoded, err := common.Marshal(document)
	if err != nil {
		return body
	}
	return encoded
}
