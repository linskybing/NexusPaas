package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	defaultMaxAPIBodyBytes        = 10 * 1024 * 1024
	defaultMaxConfigFileBytes     = 1024 * 1024
	defaultMaxConfigFileDocuments = 50
)

type InputLimitError struct {
	Status  int
	Message string
}

func (e InputLimitError) Error() string {
	return e.Message
}

func InputLimitStatus(err error, fallback int) int {
	var limitErr InputLimitError
	if errors.As(err, &limitErr) {
		return limitErr.Status
	}
	return fallback
}

func InputLimitMessage(err error, fallback string) string {
	var limitErr InputLimitError
	if errors.As(err, &limitErr) {
		return limitErr.Message
	}
	return fallback
}

func ValidateManifestLimits(raw []byte, maxBytes, maxDocuments int) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if err := validateManifestBytes(raw, maxBytes); err != nil {
		return err
	}
	return validateManifestDocuments(raw, maxDocuments)
}

func validateManifestBytes(raw []byte, maxBytes int) error {
	if maxBytes > 0 && len(raw) > maxBytes {
		return InputLimitError{Status: http.StatusRequestEntityTooLarge, Message: fmt.Sprintf("manifest exceeds max byte size of %d bytes", maxBytes)}
	}
	return nil
}

func validateManifestDocuments(raw []byte, maxDocuments int) error {
	if maxDocuments <= 0 {
		return nil
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	documents := 0
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if len(doc) == 0 {
			continue
		}
		documents++
		if documents > maxDocuments {
			return InputLimitError{Status: http.StatusUnprocessableEntity, Message: fmt.Sprintf("manifest document count exceeds limit of %d", maxDocuments)}
		}
	}
	return nil
}

func ValidateManifestValue(value any, maxBytes, maxDocuments int) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		raw := bytes.TrimSpace([]byte(typed))
		if len(raw) == 0 {
			return nil
		}
		if err := validateManifestBytes(raw, maxBytes); err != nil {
			return err
		}
		if !looksLikeManifestDocument(raw) {
			return nil
		}
		return validateManifestDocuments(raw, maxDocuments)
	case []byte:
		return ValidateManifestLimits(typed, maxBytes, maxDocuments)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		return ValidateManifestLimits(raw, maxBytes, maxDocuments)
	}
}

func looksLikeManifestDocument(raw []byte) bool {
	text := string(raw)
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") || strings.HasPrefix(text, "---") || strings.Contains(text, "\n---") {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apiVersion:") || strings.HasPrefix(line, "kind:") || strings.HasPrefix(line, "metadata:") {
			return true
		}
	}
	return false
}
