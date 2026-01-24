package middleware

import (
	"net/http"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/audit"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// getClientIP извлекает IP адрес из запроса
func getClientIP(r *http.Request) string {
	// Пробуем получить из X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Пробуем получить из X-Forwarded-For
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Берем первый IP из списка
		if ips := strings.Split(forwarded, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Возвращаем RemoteAddr (может содержать порт)
	return r.RemoteAddr
}

// WithAudit middleware для аудита запросов
func WithAudit(auditSubject audit.Subject) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Создаем кастомный ResponseWriter для перехвата статуса
			wrapped := &auditResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Пропускаем запрос через следующий обработчик
			next.ServeHTTP(wrapped, r)

			// Аудируем только успешные POST запросы на обновление метрик
			if r.Method == http.MethodPost &&
				(strings.HasPrefix(r.URL.Path, "/update") || r.URL.Path == "/updates") {

				// Аудируем только успешные запросы (2xx)
				status := wrapped.statusCode
				if status >= 200 && status < 300 {
					// Извлекаем метрики из запроса
					metrics := extractMetrics(r)

					if len(metrics) > 0 {
						// Создаем событие аудита
						event := audit.NewEvent(metrics, getClientIP(r))

						// Уведомляем наблюдателей
						auditSubject.Notify(event)

						logger.Log.Debug("Audit event sent",
							zap.Strings("metrics", metrics),
							zap.String("ip", getClientIP(r)))
					}
				}
			}
		})
	}
}

// auditResponseWriter обертка для перехвата статуса ответа
// Используется для аудита
type auditResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (rw *auditResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.wroteHeader = true
	}
}

func (rw *auditResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Status возвращает статус код ответа
func (rw *auditResponseWriter) Status() int {
	return rw.statusCode
}

// extractMetrics извлекает имена метрик из запроса
func extractMetrics(r *http.Request) []string {
	metrics := []string{}

	// Для простых запросов с URL параметрами
	if metricName := chi.URLParam(r, "metric"); metricName != "" {
		metrics = append(metrics, metricName)
	}

	// Для JSON запросов с путями /update и /updates
	// Поскольку тело уже прочитано, мы не можем извлечь метрики из JSON
	// В реальном проекте можно было бы использовать буферизацию тела

	// Для пути /updates (батч) мы не знаем имена метрик из URL
	// Это можно решить через middleware, которое кеширует тело

	return metrics
}
