package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestTrustedSubnetMiddleware_EmptySubnet(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Применяем middleware с пустой подсетью
	middleware := TrustedSubnetMiddleware("")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
}

func TestTrustedSubnetMiddleware_ValidIP(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Применяем middleware с доверенной подсетью 192.168.1.0/24
	middleware := TrustedSubnetMiddleware("192.168.1.0/24")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос с IP из доверенной подсети
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
}

func TestTrustedSubnetMiddleware_InvalidIP(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for invalid IP")
	})

	// Применяем middleware с доверенной подсетью 192.168.1.0/24
	middleware := TrustedSubnetMiddleware("192.168.1.0/24")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос с IP не из доверенной подсети
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "10.0.0.100")
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "Forbidden")
}

func TestTrustedSubnetMiddleware_MissingHeader(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when X-Real-IP header is missing")
	})

	// Применяем middleware с доверенной подсетью
	middleware := TrustedSubnetMiddleware("192.168.1.0/24")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос без заголовка X-Real-IP
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "Forbidden")
}

func TestTrustedSubnetMiddleware_InvalidCIDR(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid CIDR")
	})

	// Применяем middleware с некорректной CIDR
	middleware := TrustedSubnetMiddleware("invalid-cidr")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат - должна быть внутренняя ошибка сервера
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestTrustedSubnetMiddleware_InvalidIPFormat(t *testing.T) {
	// Создаем тестовый обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid IP format")
	})

	// Применяем middleware с доверенной подсетью
	middleware := TrustedSubnetMiddleware("192.168.1.0/24")
	wrappedHandler := middleware(handler)

	// Создаем тестовый запрос с некорректным форматом IP
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "not-an-ip")
	rr := httptest.NewRecorder()

	// Выполняем запрос
	wrappedHandler.ServeHTTP(rr, req)

	// Проверяем результат
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "Forbidden")
}

func TestGetRealIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:     "X-Real-IP present",
			headers:  map[string]string{"X-Real-IP": "192.168.1.100"},
			expected: "192.168.1.100",
		},
		{
			name:     "X-Forwarded-For present",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.100"},
			expected: "10.0.0.1",
		},
		{
			name: "Both headers present, X-Real-IP takes precedence",
			headers: map[string]string{
				"X-Real-IP":       "192.168.1.100",
				"X-Forwarded-For": "10.0.0.1",
			},
			expected: "192.168.1.100",
		},
		{
			name:       "No headers, use RemoteAddr",
			remoteAddr: "10.0.0.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "No headers, invalid RemoteAddr format",
			remoteAddr: "invalid-addr",
			expected:   "invalid-addr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			if tt.remoteAddr != "" {
				req.RemoteAddr = tt.remoteAddr
			}

			result := GetRealIP(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrustedSubnetMiddleware_Integration тестирует интеграцию middleware с роутером chi
func TestTrustedSubnetMiddleware_Integration(t *testing.T) {
	r := chi.NewRouter()

	// Применяем middleware к роутеру
	r.Use(TrustedSubnetMiddleware("192.168.1.0/24"))

	// Добавляем тестовый эндпоинт
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	tests := []struct {
		name         string
		realIP       string
		expectedCode int
		expectedBody string
	}{
		{
			name:         "Valid IP in trusted subnet",
			realIP:       "192.168.1.100",
			expectedCode: http.StatusOK,
			expectedBody: "ok",
		},
		{
			name:         "Invalid IP not in trusted subnet",
			realIP:       "10.0.0.100",
			expectedCode: http.StatusForbidden,
			expectedBody: "Forbidden\n",
		},
		{
			name:         "Missing X-Real-IP header",
			realIP:       "",
			expectedCode: http.StatusForbidden,
			expectedBody: "Forbidden\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.Equal(t, tt.expectedBody, rr.Body.String())
		})
	}
}
