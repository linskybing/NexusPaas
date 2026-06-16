package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type Envelope struct {
	Success   bool       `json:"success"`
	Data      any        `json:"data,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
	Degraded  *Degraded  `json:"degraded,omitempty"`
	RequestID string     `json:"request_id"`
	TraceID   string     `json:"trace_id"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Degraded struct {
	Adapter   string `json:"adapter"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type RawResponse struct {
	ContentType  string
	Headers      map[string]string
	HeaderValues map[string][]string
	Body         []byte
}

const contentTypeJSON = "application/json"

func WriteJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	if raw, ok := data.(RawResponse); ok {
		for key, value := range raw.Headers {
			w.Header().Set(key, value)
		}
		for key, values := range raw.HeaderValues {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		if raw.ContentType != "" {
			w.Header().Set(headerContentType, raw.ContentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write(raw.Body)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success:   status < 400,
		Data:      data,
		RequestID: RequestID(r),
		TraceID:   TraceID(r),
	})
}

func WriteDegraded(w http.ResponseWriter, r *http.Request, status int, data any, degraded Degraded) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success:   status < 400,
		Data:      data,
		Degraded:  &degraded,
		RequestID: RequestID(r),
		TraceID:   TraceID(r),
	})
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
		RequestID: RequestID(r),
		TraceID:   TraceID(r),
	})
}

func DecodeMap(r *http.Request) map[string]any {
	payload, err := DecodeMapWithError(r)
	if err != nil {
		return map[string]any{}
	}
	return payload
}

func DecodeMapWithError(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}
