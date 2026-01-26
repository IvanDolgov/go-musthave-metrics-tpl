package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWithGzipEdgeCases тестирует граничные случаи gzip middleware
func TestWithGzipEdgeCases(t *testing.T) {
	t.Run("Empty response body", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пустой ответ
			w.WriteHeader(http.StatusNoContent)
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, rr.Code)
		}
	})

	t.Run("Large response compression", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Большой ответ для тестирования сжатия
			data := strings.Repeat("Hello, World! ", 1000)
			w.Write([]byte(data))
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Header().Get("Content-Encoding") != "gzip" {
			t.Error("Expected gzip encoding for large response")
		}
	})

	t.Run("Invalid gzip request body", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := WithGzip(handler)
		// Невалидные gzip данные
		req := httptest.NewRequest("POST", "/", strings.NewReader("not-gzip-data"))
		req.Header.Set("Content-Encoding", "gzip")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for invalid gzip, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("Multiple encodings in Accept-Encoding", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("test"))
		})

		wrapped := WithGzip(handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")

		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		if rr.Header().Get("Content-Encoding") != "gzip" {
			t.Error("Expected gzip encoding when multiple encodings accepted")
		}
	})
}

// TestWithGzipConcurrent тестирует конкурентное использование gzip middleware
func TestWithGzipConcurrent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("concurrent test"))
	})

	wrapped := WithGzip(handler)

	// Запускаем несколько горутин для конкурентного доступа
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Goroutine %d: Expected status %d, got %d", id, http.StatusOK, rr.Code)
			}
			done <- true
		}(i)
	}

	// Ждем завершения всех горутин
	for i := 0; i < 10; i++ {
		<-done
	}
}
