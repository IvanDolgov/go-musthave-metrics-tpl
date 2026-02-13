package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithGzip(t *testing.T) {
	tests := []struct {
		name               string
		acceptEncoding     string
		contentEncoding    string
		requestBody        string
		expectedStatusCode int
		checkCompression   bool
	}{
		{
			name:               "Client accepts gzip",
			acceptEncoding:     "gzip",
			contentEncoding:    "",
			requestBody:        "",
			expectedStatusCode: http.StatusOK,
			checkCompression:   true,
		},
		{
			name:               "Client does not accept gzip",
			acceptEncoding:     "",
			contentEncoding:    "",
			requestBody:        "",
			expectedStatusCode: http.StatusOK,
			checkCompression:   false,
		},
		{
			name:               "Client sends gzip compressed data",
			acceptEncoding:     "",
			contentEncoding:    "gzip",
			requestBody:        "test data",
			expectedStatusCode: http.StatusOK,
			checkCompression:   false,
		},
		{
			name:               "Client accepts gzip and sends gzip",
			acceptEncoding:     "gzip",
			contentEncoding:    "gzip",
			requestBody:        "compressed test data",
			expectedStatusCode: http.StatusOK,
			checkCompression:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.contentEncoding == "gzip" && tt.requestBody != "" {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Equal(t, tt.requestBody, string(body))
				}
				w.Write([]byte("Hello, World!"))
			})

			wrapped := WithGzip(handler)

			var reqBody io.Reader
			if tt.requestBody != "" && tt.contentEncoding == "gzip" {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				_, err := gz.Write([]byte(tt.requestBody))
				assert.NoError(t, err)
				err = gz.Close()
				assert.NoError(t, err)
				reqBody = &buf
			} else if tt.requestBody != "" {
				reqBody = strings.NewReader(tt.requestBody)
			}

			req := httptest.NewRequest("GET", "/test", reqBody)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			if tt.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tt.contentEncoding)
			}

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatusCode, rr.Code)

			if tt.checkCompression {
				assert.Equal(t, "gzip", rr.Header().Get("Content-Encoding"))

				reader, err := gzip.NewReader(rr.Body)
				assert.NoError(t, err)
				defer reader.Close()

				decompressed, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, "Hello, World!", string(decompressed))
			} else {
				assert.Empty(t, rr.Header().Get("Content-Encoding"))
				assert.Equal(t, "Hello, World!", rr.Body.String())
			}
		})
	}
}

func TestWithGzipEdgeCases(t *testing.T) {
	t.Run("Empty response body", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("Large response compression", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data := strings.Repeat("Hello, World! ", 1000)
			_, err := w.Write([]byte(data))
			assert.NoError(t, err)
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, "gzip", rr.Header().Get("Content-Encoding"))
	})

	t.Run("Invalid gzip request body", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("POST", "/", strings.NewReader("not-gzip-data"))
		req.Header.Set("Content-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Multiple encodings in Accept-Encoding", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("test"))
			assert.NoError(t, err)
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		assert.Equal(t, "gzip", rr.Header().Get("Content-Encoding"))
	})
}

func TestWithGzipConcurrent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("concurrent test"))
		assert.NoError(t, err)
	})

	wrapped := WithGzip(handler)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
