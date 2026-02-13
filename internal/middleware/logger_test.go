package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestWithLogging(t *testing.T) {
	logger.Log = zap.NewNop()

	tests := []struct {
		name         string
		method       string
		path         string
		responseBody string
		statusCode   int
	}{
		{
			name:         "GET request",
			method:       "GET",
			path:         "/test",
			responseBody: "Hello",
			statusCode:   http.StatusOK,
		},
		{
			name:         "POST request",
			method:       "POST",
			path:         "/update",
			responseBody: "Updated",
			statusCode:   http.StatusCreated,
		},
		{
			name:         "Error response",
			method:       "GET",
			path:         "/notfound",
			responseBody: "Not Found",
			statusCode:   http.StatusNotFound,
		},
		{
			name:         "No Content response",
			method:       "DELETE",
			path:         "/delete",
			responseBody: "",
			statusCode:   http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					_, err := w.Write([]byte(tt.responseBody))
					assert.NoError(t, err)
				}
			})

			wrapped := WithLogging(handler)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			assert.Equal(t, tt.statusCode, rr.Code)
			assert.Equal(t, tt.responseBody, rr.Body.String())
		})
	}
}

func TestLoggingResponseWriter(t *testing.T) {
	t.Run("WriteHeader sets status code", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
			statusCode:     http.StatusOK,
		}

		rw.WriteHeader(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, rw.statusCode)
		assert.Equal(t, http.StatusNotFound, mockWriter.Code)
	})

	t.Run("Write without WriteHeader", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
			statusCode:     http.StatusOK,
		}

		data := []byte("test")
		n, err := rw.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, http.StatusOK, rw.statusCode)
		assert.Equal(t, http.StatusOK, mockWriter.Code)
		assert.Equal(t, len(data), rw.responseSize)
	})

	t.Run("Multiple Write calls accumulate size", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
			statusCode:     http.StatusOK,
		}

		_, _ = rw.Write([]byte("Hello "))
		_, _ = rw.Write([]byte("World!"))

		assert.Equal(t, 12, rw.responseSize)
	})

	t.Run("Multiple WriteHeader calls", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
			statusCode:     http.StatusOK,
		}

		rw.WriteHeader(http.StatusBadRequest)
		rw.WriteHeader(http.StatusInternalServerError)

		assert.Equal(t, http.StatusBadRequest, rw.statusCode)
		assert.Equal(t, http.StatusBadRequest, mockWriter.Code)
	})

	t.Run("Status and Size methods", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
			statusCode:     http.StatusOK,
		}

		assert.Equal(t, http.StatusOK, rw.Status())
		assert.Equal(t, 0, rw.Size())

		rw.WriteHeader(http.StatusCreated)
		assert.Equal(t, http.StatusCreated, rw.Status())

		_, _ = rw.Write([]byte("test"))
		assert.Equal(t, 4, rw.Size())
	})

	t.Run("Header method passthrough", func(t *testing.T) {
		mockWriter := httptest.NewRecorder()
		rw := &loggingResponseWriter{
			ResponseWriter: mockWriter,
		}

		rw.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "application/json", mockWriter.Header().Get("Content-Type"))
	})
}

func TestEmptyHandlerWithLogging(t *testing.T) {
	logger.Log = zap.NewNop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Пустой handler
	})

	wrapped := WithLogging(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandlerThatOnlyWritesHeader(t *testing.T) {
	logger.Log = zap.NewNop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	wrapped := WithLogging(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String())
}
