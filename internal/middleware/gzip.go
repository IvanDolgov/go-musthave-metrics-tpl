package middleware

import (
	"net/http"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
)

// WithGzip добавляет сжатие данных для HTTP запросов и ответов.
// Middleware выполняет две функции:
//  1. Распаковывает входящие запросы, сжатые в формате gzip
//  2. Сжимает исходящие ответы, если клиент поддерживает gzip
//
// Алгоритм работы:
//   - Если запрос содержит заголовок "Content-Encoding: gzip", тело распаковывается
//   - Если запрос содержит заголовок "Accept-Encoding" со значением "gzip",
//     ответ сжимается перед отправкой клиенту
//
// Пример использования:
//
//	r.Use(middleware.WithGzip)
//
// Параметры:
//   - h: следующий обработчик в цепочке middleware
//
// Возвращает:
//   - http.Handler: обработчик с поддержкой сжатия gzip
func WithGzip(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		// Всегда распаковываем входящие сжатые данные
		if r.Header.Get("Content-Encoding") == "gzip" {
			cr, err := compress.NewCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.Body = cr
			defer cr.Close()
		}

		// Всегда сжимаем исходящие данные если клиент поддерживает
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			cw := compress.NewCompressWriter(w)
			ow = cw
			defer cw.Close()
		}

		h.ServeHTTP(ow, r)
	})
}
