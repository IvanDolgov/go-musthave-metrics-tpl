package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHashValidation(t *testing.T) {
	logger.Log = zap.NewNop()

	key := "test-secret-key"
	testBody := `{"id":"test","type":"gauge","value":42.5}`
	correctHash := hash.ComputeHMACSHA256([]byte(testBody), key)

	tests := []struct {
		name           string
		key            string
		requestBody    string
		hashHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid hash with key",
			key:            key,
			requestBody:    testBody,
			hashHeader:     correctHash,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid hash with key",
			key:            key,
			requestBody:    testBody,
			hashHeader:     "invalid-hash",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "No hash with empty key",
			key:            "",
			requestBody:    testBody,
			hashHeader:     "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No hash with key",
			key:            key,
			requestBody:    testBody,
			hashHeader:     "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty request body with valid hash",
			key:            key,
			requestBody:    "",
			hashHeader:     hash.ComputeHMACSHA256([]byte(""), key),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Very large body",
			key:            key,
			requestBody:    strings.Repeat("x", 10000),
			hashHeader:     hash.ComputeHMACSHA256([]byte(strings.Repeat("x", 10000)), key),
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("OK"))
				assert.NoError(t, err)
			})

			middleware := HashValidation(tt.key)
			wrapped := middleware(handler)

			req := httptest.NewRequest("POST", "/test", strings.NewReader(tt.requestBody))
			if tt.hashHeader != "" {
				req.Header.Set("HashSHA256", tt.hashHeader)
			}

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "OK", rr.Body.String())
			}
		})
	}
}

func TestHashResponse(t *testing.T) {
	key := "test-secret-key"
	responseBody := `{"status":"ok"}`

	tests := []struct {
		name       string
		key        string
		response   string
		expectHash bool
	}{
		{
			name:       "With key and response",
			key:        key,
			response:   responseBody,
			expectHash: true,
		},
		{
			name:       "With empty key",
			key:        "",
			response:   responseBody,
			expectHash: false,
		},
		{
			name:       "With key but empty response",
			key:        key,
			response:   "",
			expectHash: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(tt.response))
				assert.NoError(t, err)
			})

			middleware := HashResponse(tt.key)
			wrapped := middleware(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrapped.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)

			hashHeader := rr.Header().Get("HashSHA256")
			if tt.expectHash {
				assert.NotEmpty(t, hashHeader)
				expectedHash := hash.ComputeHMACSHA256([]byte(tt.response), tt.key)
				assert.Equal(t, expectedHash, hashHeader)
			} else {
				assert.Empty(t, hashHeader)
			}

			assert.Equal(t, tt.response, rr.Body.String())
		})
	}
}

func TestHashResponseWriter(t *testing.T) {
	t.Run("WriteHeader computes hash before sending headers", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		hw := &hashResponseWriter{
			ResponseWriter: mockWriter,
			key:            "test-key",
		}

		responseBody := []byte("test response")
		_, err := hw.Write(responseBody)
		assert.NoError(t, err)

		hw.WriteHeader(http.StatusCreated)

		hashHeader := mockWriter.Header().Get("HashSHA256")
		assert.NotEmpty(t, hashHeader)
		assert.Equal(t, http.StatusCreated, mockWriter.Code)
	})

	t.Run("Header method passthrough", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		hw := &hashResponseWriter{
			ResponseWriter: mockWriter,
			key:            "test",
		}

		hw.Header().Set("X-Test", "value")
		assert.Equal(t, "value", mockWriter.Header().Get("X-Test"))
		assert.Equal(t, "test", hw.key)
	})
}
