package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/audit"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockAuditSubject - мок для audit.Subject
type MockAuditSubject struct {
	notifyCount int
	lastEvent   *audit.Event
}

func (m *MockAuditSubject) Register(observer audit.Observer) {
	// Заглушка для тестов
}

func (m *MockAuditSubject) Deregister(observer audit.Observer) {
	// Заглушка для тестов
}

func (m *MockAuditSubject) Notify(event *audit.Event) {
	m.notifyCount++
	m.lastEvent = event
}

func (m *MockAuditSubject) Start() {
	// Заглушка для тестов
}

func (m *MockAuditSubject) Stop() {
	// Заглушка для тестов
}

func (m *MockAuditSubject) Reset() {
	m.notifyCount = 0
	m.lastEvent = nil
}

// TestGetClientIP тестирует извлечение IP адреса
func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "X-Real-IP header",
			headers:    map[string]string{"X-Real-IP": "192.168.1.100"},
			remoteAddr: "10.0.0.1:8080",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.2"},
			remoteAddr: "10.0.0.1:8080",
			expectedIP: "10.0.0.2",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178"},
			remoteAddr: "10.0.0.1:8080",
			expectedIP: "203.0.113.195",
		},
		{
			name: "Both headers, X-Real-IP priority",
			headers: map[string]string{
				"X-Real-IP":       "192.168.1.100",
				"X-Forwarded-For": "10.0.0.2",
			},
			remoteAddr: "10.0.0.1:8080",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "No headers, use RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "172.16.0.5:1234",
			expectedIP: "172.16.0.5:1234",
		},
		{
			name:       "No headers, RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "172.16.0.5",
			expectedIP: "172.16.0.5",
		},
		{
			name:       "Empty headers",
			headers:    map[string]string{"X-Real-IP": "", "X-Forwarded-For": ""},
			remoteAddr: "10.0.0.1:8080",
			expectedIP: "10.0.0.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				if value != "" {
					req.Header.Set(key, value)
				}
			}

			ip := getClientIP(req)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

// TestAuditResponseWriter тестирует обертку ResponseWriter
func TestAuditResponseWriter(t *testing.T) {
	t.Run("WriteHeader sets status code", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &auditResponseWriter{
			ResponseWriter: recorder,
			statusCode:     http.StatusOK,
		}

		rw.WriteHeader(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, rw.statusCode)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("Write without WriteHeader", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &auditResponseWriter{
			ResponseWriter: recorder,
			statusCode:     http.StatusOK,
		}

		data := []byte("test response")
		n, err := rw.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, http.StatusOK, rw.statusCode)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "test response", recorder.Body.String())
	})

	t.Run("Multiple WriteHeader calls", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &auditResponseWriter{
			ResponseWriter: recorder,
			statusCode:     http.StatusOK,
		}

		rw.WriteHeader(http.StatusBadRequest)
		rw.WriteHeader(http.StatusInternalServerError) // Должен игнорироваться

		assert.Equal(t, http.StatusBadRequest, rw.statusCode)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("Status method", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &auditResponseWriter{
			ResponseWriter: recorder,
			statusCode:     http.StatusOK,
		}

		assert.Equal(t, http.StatusOK, rw.Status())

		rw.WriteHeader(http.StatusCreated)
		assert.Equal(t, http.StatusCreated, rw.Status())
	})

	t.Run("Header passthrough", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := &auditResponseWriter{
			ResponseWriter: recorder,
		}

		rw.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	})
}

// setupChiContext - вспомогательная функция для создания chi контекста
func setupChiContext(r *http.Request, metricName string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("metric", metricName)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestExtractMetrics тестирует извлечение метрик из запроса
func TestExtractMetrics(t *testing.T) {
	t.Run("URL parameter metric", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/update/gauge/TestMetric/100", nil)
		r = setupChiContext(r, "TestMetric")

		metrics := extractMetrics(r, nil)
		assert.Equal(t, []string{"TestMetric"}, metrics)
	})

	t.Run("Single metric JSON update", func(t *testing.T) {
		metric := models.Metrics{
			ID:    "TestGauge",
			MType: "gauge",
			Value: func() *float64 { v := 42.5; return &v }(),
		}

		body, err := json.Marshal(metric)
		require.NoError(t, err)

		r := httptest.NewRequest("POST", "/update", bytes.NewReader(body))
		r.URL.Path = "/update"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Equal(t, []string{"TestGauge"}, metrics)
	})

	t.Run("Single metric JSON update with trailing slash", func(t *testing.T) {
		metric := models.Metrics{
			ID:    "TestCounter",
			MType: "counter",
			Delta: func() *int64 { v := int64(10); return &v }(),
		}

		body, err := json.Marshal(metric)
		require.NoError(t, err)

		r := httptest.NewRequest("POST", "/update/", bytes.NewReader(body))
		r.URL.Path = "/update/"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Equal(t, []string{"TestCounter"}, metrics)
	})

	t.Run("Batch metrics JSON update", func(t *testing.T) {
		metrics := []models.Metrics{
			{
				ID:    "Gauge1",
				MType: "gauge",
				Value: func() *float64 { v := 10.5; return &v }(),
			},
			{
				ID:    "Counter1",
				MType: "counter",
				Delta: func() *int64 { v := int64(5); return &v }(),
			},
		}

		body, err := json.Marshal(metrics)
		require.NoError(t, err)

		r := httptest.NewRequest("POST", "/updates", bytes.NewReader(body))
		r.URL.Path = "/updates"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		result := extractMetrics(r, bufferedReq)
		assert.Equal(t, []string{"Gauge1", "Counter1"}, result)
	})

	t.Run("Invalid JSON single metric", func(t *testing.T) {
		body := []byte(`{"invalid": "json"}`)
		r := httptest.NewRequest("POST", "/update", bytes.NewReader(body))
		r.URL.Path = "/update"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Empty(t, metrics)
	})

	t.Run("Invalid JSON batch metrics", func(t *testing.T) {
		body := []byte(`[{"invalid": "json"}]`)
		r := httptest.NewRequest("POST", "/updates", bytes.NewReader(body))
		r.URL.Path = "/updates"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Empty(t, metrics)
	})

	t.Run("Empty buffer", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/update", nil)
		r.URL.Path = "/update"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer([]byte{}),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Empty(t, metrics)
	})

	t.Run("Nil buffered request", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/update/gauge/Test/100", nil)
		r = setupChiContext(r, "Test")

		metrics := extractMetrics(r, nil)
		assert.Equal(t, []string{"Test"}, metrics)
	})

	t.Run("Non-update path", func(t *testing.T) {
		metric := models.Metrics{
			ID:    "Test",
			MType: "gauge",
			Value: func() *float64 { v := 42.0; return &v }(),
		}

		body, err := json.Marshal(metric)
		require.NoError(t, err)

		r := httptest.NewRequest("POST", "/some-other-path", bytes.NewReader(body))
		r.URL.Path = "/some-other-path"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		metrics := extractMetrics(r, bufferedReq)
		assert.Empty(t, metrics)
	})
}

// TestWithAuditMiddleware тестирует middleware аудита
func TestWithAuditMiddleware(t *testing.T) {
	// Инициализируем логер для тестов
	logger.Log = zap.NewNop()

	t.Run("POST /update with valid metric - success", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		metric := models.Metrics{
			ID:    "TestMetric",
			MType: "gauge",
			Value: func() *float64 { v := 123.45; return &v }(),
		}

		body, _ := json.Marshal(metric)
		req := httptest.NewRequest("POST", "/update", bytes.NewReader(body))
		req.RemoteAddr = "192.168.1.100:8080"

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, 1, mockSubject.notifyCount)
		assert.NotNil(t, mockSubject.lastEvent)
	})

	t.Run("POST /updates with batch - success", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		metrics := []models.Metrics{
			{ID: "Metric1", MType: "gauge", Value: func() *float64 { v := 1.0; return &v }()},
			{ID: "Metric2", MType: "counter", Delta: func() *int64 { v := int64(2); return &v }()},
		}

		body, _ := json.Marshal(metrics)
		req := httptest.NewRequest("POST", "/updates", bytes.NewReader(body))
		req.RemoteAddr = "10.0.0.1:5678"
		req.Header.Set("X-Real-IP", "203.0.113.1")

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, 1, mockSubject.notifyCount)
		assert.NotNil(t, mockSubject.lastEvent)
	})

	t.Run("POST /update with URL parameter - success", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		req := httptest.NewRequest("POST", "/update/gauge/TestMetric/100", nil)
		req.RemoteAddr = "10.0.0.1:8080"
		req = setupChiContext(req, "TestMetric")

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, 1, mockSubject.notifyCount)
	})

	t.Run("POST with error response - no audit", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		metric := models.Metrics{ID: "Test", MType: "gauge", Value: func() *float64 { v := 1.0; return &v }()}
		body, _ := json.Marshal(metric)
		req := httptest.NewRequest("POST", "/update", bytes.NewReader(body))

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, 0, mockSubject.notifyCount)
	})

	t.Run("GET request - no audit", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		req := httptest.NewRequest("GET", "/value/gauge/TestMetric", nil)
		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, 0, mockSubject.notifyCount)
	})

	t.Run("POST to non-update path - no audit", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		req := httptest.NewRequest("POST", "/some-other-path", nil)
		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, 0, mockSubject.notifyCount)
	})

	t.Run("Failed to read body", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		badReader := &brokenReader{}
		req := httptest.NewRequest("POST", "/update", badReader)

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, 0, mockSubject.notifyCount)
	})

	t.Run("Invalid JSON in body", func(t *testing.T) {
		mockSubject := &MockAuditSubject{}

		req := httptest.NewRequest("POST", "/update", strings.NewReader("{invalid json}"))

		recorder := httptest.NewRecorder()

		handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))

		handler.ServeHTTP(recorder, req)

		assert.Equal(t, 0, mockSubject.notifyCount)
	})
}

// TestAuditMiddlewareWithEmptySubject тестирует middleware с nil subject
func TestAuditMiddlewareWithEmptySubject(t *testing.T) {
	t.Run("nil audit subject", func(t *testing.T) {
		assert.NotPanics(t, func() {
			handler := WithAudit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("POST", "/update", nil)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)
		})
	})
}

// TestBufferedRequest тестирует структуру bufferedRequest
func TestBufferedRequest(t *testing.T) {
	t.Run("body can be read multiple times", func(t *testing.T) {
		originalBody := []byte("test body")
		buffer := bytes.NewBuffer(originalBody)

		req := httptest.NewRequest("POST", "/update", nil)
		bufferedReq := &bufferedRequest{
			Request:    req,
			bodyBuffer: buffer,
		}
		bufferedReq.Body = io.NopCloser(buffer)

		body1, err := io.ReadAll(bufferedReq.Body)
		require.NoError(t, err)
		assert.Equal(t, originalBody, body1)

		bufferedReq.Body = io.NopCloser(bufferedReq.bodyBuffer)
		body2, err := io.ReadAll(bufferedReq.Body)
		require.NoError(t, err)
		assert.Equal(t, originalBody, body2)
	})
}

// Вспомогательные типы для тестов
type brokenReader struct{}

func (b *brokenReader) Read(p []byte) (n int, err error) {
	return 0, assert.AnError
}

func (b *brokenReader) Close() error {
	return nil
}

// BenchmarkGetClientIP бенчмарк для getClientIP
func BenchmarkGetClientIP(b *testing.B) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	req.RemoteAddr = "172.16.0.1:8080"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getClientIP(req)
	}
}

// BenchmarkExtractMetrics бенчмарк для extractMetrics
func BenchmarkExtractMetrics(b *testing.B) {
	metric := models.Metrics{
		ID:    "BenchmarkMetric",
		MType: "gauge",
		Value: func() *float64 { v := 123.45; return &v }(),
	}

	body, _ := json.Marshal(metric)

	b.Run("Single metric", func(b *testing.B) {
		r := httptest.NewRequest("POST", "/update", bytes.NewReader(body))
		r.URL.Path = "/update"

		bufferedReq := &bufferedRequest{
			Request:    r,
			bodyBuffer: bytes.NewBuffer(body),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			extractMetrics(r, bufferedReq)
		}
	})

	b.Run("URL parameter", func(b *testing.B) {
		r := httptest.NewRequest("POST", "/update/gauge/Test/100", nil)
		r = setupChiContext(r, "Test")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			extractMetrics(r, nil)
		}
	})
}

// BenchmarkAuditMiddleware бенчмарк для middleware
func BenchmarkAuditMiddleware(b *testing.B) {
	mockSubject := &MockAuditSubject{}
	metric := models.Metrics{
		ID:    "Benchmark",
		MType: "gauge",
		Value: func() *float64 { v := 123.45; return &v }(),
	}

	body, _ := json.Marshal(metric)

	handler := WithAudit(mockSubject)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/update", bytes.NewReader(body))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
	}
}
