// Package middleware предоставляет HTTP middleware компоненты для сервера метрик.
// Включает middleware для логирования, сжатия данных, проверки хешей и других функций.
package middleware

import (
	"net/http"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// WithLogging добавляет логирование для всех HTTP запросов и ответов.
// Регистрирует информацию о каждом запросе: URI, метод, IP адрес клиента,
// статус код ответа, размер ответа и время выполнения.
//
// Пример лога:
//
//	HTTP request processed {"uri": "/update/gauge/cpu_usage/42.5", "method": "POST",
//	"remote_addr": "192.168.1.1:12345", "status_code": 200, "response_size": 45,
//	"duration": "12.345ms"}
//
// Параметры:
//   - h: следующий обработчик в цепочке middleware
//
// Возвращает:
//   - http.Handler: обработчик с добавленным логированием
func WithLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// засекаем время начала обработки запроса
		start := time.Now()

		// Создаем кастомный ResponseWriter для перехвата статуса и размера ответа
		wrapped := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Обслуживаем запрос
		h.ServeHTTP(wrapped, r)

		// Вычисляем продолжительность выполнения
		duration := time.Since(start)

		// Логируем сведения о запросе и ответе на уровне Info
		logger.Log.Info("HTTP request processed",
			zap.String("uri", r.RequestURI),
			zap.String("method", r.Method),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Int("status_code", wrapped.statusCode),
			zap.Int("response_size", wrapped.responseSize),
			zap.Duration("duration", duration),
		)
	})
}

// loggingResponseWriter - обертка для http.ResponseWriter для перехвата статуса и размера ответа.
// Используется в middleware WithLogging для сбора метрик о HTTP ответах.
//
// Реализует интерфейс http.ResponseWriter с дополнительными методами:
//   - Status(): возвращает статус код ответа
//   - Size(): возвращает размер тела ответа в байтах
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int
	wroteHeader  bool
}

// WriteHeader перехватывает вызов WriteHeader для записи статус кода.
// Гарантирует, что статус код записывается только один раз.
func (rw *loggingResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.wroteHeader = true
	}
}

// Write перехватывает запись данных для подсчета размера ответа.
// Гарантирует, что заголовки отправляются перед телом ответа.
func (rw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.responseSize += size
	return size, err
}

// Status возвращает статус код ответа.
// Используется для логирования после обработки запроса.
func (rw *loggingResponseWriter) Status() int {
	return rw.statusCode
}

// Size возвращает размер ответа в байтах.
// Используется для логирования после обработки запроса.
func (rw *loggingResponseWriter) Size() int {
	return rw.responseSize
}
