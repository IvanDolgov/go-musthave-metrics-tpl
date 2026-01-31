package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/audit"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
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

// bufferedRequest хранит буферизованный запрос
type bufferedRequest struct {
	*http.Request
	bodyBuffer *bytes.Buffer
}

// WithAudit middleware для аудита запросов
func WithAudit(auditSubject audit.Subject) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Буферизуем тело только для POST запросов на обновление метрик
			var bufferedReq *bufferedRequest
			if r.Method == http.MethodPost &&
				(strings.HasPrefix(r.URL.Path, "/update") || r.URL.Path == "/updates") {

				// Копируем тело запроса в буфер
				bodyBytes, err := io.ReadAll(r.Body)
				if err != nil {
					logger.Log.Error("Failed to read request body", zap.Error(err))
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				// Закрываем оригинальное тело
				r.Body.Close()

				// Создаем новый буфер и заменяем тело запроса
				bodyBuffer := bytes.NewBuffer(bodyBytes)
				bufferedReq = &bufferedRequest{
					Request:    r,
					bodyBuffer: bodyBuffer,
				}
				bufferedReq.Body = io.NopCloser(bodyBuffer)

				// Используем буферизованный запрос
				r = bufferedReq.Request
			}

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
					metrics := extractMetrics(r, bufferedReq)

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
func extractMetrics(r *http.Request, bufferedReq *bufferedRequest) []string {
	metrics := []string{}

	// Для простых запросов с URL параметрами
	if metricName := chi.URLParam(r, "metric"); metricName != "" {
		metrics = append(metrics, metricName)
	}

	// Для JSON запросов с путем /update
	if bufferedReq != nil && bufferedReq.bodyBuffer != nil {
		// Парсим JSON из буфера
		if strings.HasSuffix(r.URL.Path, "/update") && !strings.Contains(r.URL.Path, "/updates") {
			// Одиночный update
			var metric models.Metrics
			bodyBytes := bufferedReq.bodyBuffer.Bytes()
			decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
			if err := decoder.Decode(&metric); err == nil && metric.ID != "" {
				metrics = append(metrics, metric.ID)
			}
		} else if r.URL.Path == "/updates" {
			// Батч updates
			var batchMetrics []models.Metrics
			bodyBytes := bufferedReq.bodyBuffer.Bytes()
			decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
			if err := decoder.Decode(&batchMetrics); err == nil {
				for _, metric := range batchMetrics {
					if metric.ID != "" {
						metrics = append(metrics, metric.ID)
					}
				}
			}
		}
	}

	return metrics
}
