package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type requestMetadata struct {
	RequestID     string
	CorrelationID string
}

type requestMetadataKey struct{}

func withRequestMetadata(r *http.Request) (*http.Request, requestMetadata) {
	requestID := newRequestID()
	correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if !validCorrelationID(correlationID) {
		correlationID = requestID
	}

	meta := requestMetadata{
		RequestID:     requestID,
		CorrelationID: correlationID,
	}

	ctx := context.WithValue(r.Context(), requestMetadataKey{}, meta)
	return r.WithContext(ctx), meta
}

func metadataFromContext(ctx context.Context) requestMetadata {
	if meta, ok := ctx.Value(requestMetadataKey{}).(requestMetadata); ok {
		return meta
	}
	return requestMetadata{}
}

func requestIDFromContext(ctx context.Context) string {
	return metadataFromContext(ctx).RequestID
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func validCorrelationID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}
