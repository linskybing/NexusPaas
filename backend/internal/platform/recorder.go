package platform

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

type rawResponseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newRawResponseRecorder() *rawResponseRecorder {
	return &rawResponseRecorder{header: http.Header{}}
}

func (r *rawResponseRecorder) Header() http.Header {
	return r.header
}

func (r *rawResponseRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *rawResponseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *rawResponseRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *rawResponseRecorder) rawResponse() RawResponse {
	headers := map[string]string{}
	for key, values := range r.header {
		if len(values) == 0 || strings.EqualFold(key, headerContentType) {
			continue
		}
		headers[key] = values[0]
	}
	return RawResponse{
		ContentType: r.header.Get(headerContentType),
		Headers:     headers,
		Body:        append([]byte(nil), r.body.Bytes()...),
	}
}
