package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
)

// HashValidation middleware проверяет хеш входящих запросов.
// Используется для обеспечения целостности данных при передаче по сети.
//
// Алгоритм работы:
//  1. Читает тело запроса и вычисляет HMAC-SHA256 хеш с использованием секретного ключа
//  2. Сравнивает вычисленный хеш с хешем из заголовка "HashSHA256"
//  3. Если хеши не совпадают, возвращает ошибку 400 Bad Request
//
// Если ключ не установлен (пустая строка), middleware пропускает проверку.
//
// Пример заголовка запроса:
//
//	HashSHA256: d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592
//
// Параметры:
//   - key: секретный ключ для вычисления HMAC (если пустой - проверка отключается)
//
// Возвращает:
//   - func(http.Handler) http.Handler: функция-обертка для добавления проверки хеша
func HashValidation(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key != "" {
				// Читаем тело запроса
				body, err := io.ReadAll(r.Body)
				if err != nil {
					logger.Log.Error("Failed to read request body for hash validation")
					http.Error(w, "Bad request", http.StatusBadRequest)
					return
				}

				// Восстанавливаем тело для дальнейшей обработки
				r.Body = io.NopCloser(bytes.NewBuffer(body))

				// Получаем хеш из заголовка
				receivedHash := r.Header.Get("HashSHA256")

				// Проверяем хеш
				if !hash.VerifyHMACSHA256(body, receivedHash, key) {
					logger.Log.Warn("Hash validation failed")
					http.Error(w, "Bad request", http.StatusBadRequest)
					return
				}

				logger.Log.Debug("Hash validation successful")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// HashResponse middleware добавляет хеш к исходящим ответам.
// Вычисляет HMAC-SHA256 хеш тела ответа и добавляет его в заголовок "HashSHA256".
//
// Алгоритм работы:
//  1. Перехватывает запись ответа с помощью hashResponseWriter
//  2. Вычисляет хеш от тела ответа
//  3. Добавляет хеш в заголовок перед отправкой клиенту
//
// Если ключ не установлен (пустая строка), middleware пропускает вычисление хеша.
//
// Пример заголовка ответа:
//
//	HashSHA256: d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592
//
// Параметры:
//   - key: секретный ключ для вычисления HMAC (если пустой - вычисление отключается)
//
// Возвращает:
//   - func(http.Handler) http.Handler: функция-обертка для добавления хеша к ответам
func HashResponse(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Используем custom ResponseWriter для перехвата ответа
			hw := &hashResponseWriter{ResponseWriter: w, key: key}
			next.ServeHTTP(hw, r)

			// Вычисляем хеш и добавляем в заголовок
			if hw.buffer.Len() > 0 {
				hashValue := hash.ComputeHMACSHA256(hw.buffer.Bytes(), key)
				if hashValue != "" {
					w.Header().Set("HashSHA256", hashValue)
				}
			}
		})
	}
}

// hashResponseWriter перехватывает запись ответа для вычисления хеша.
// Реализует интерфейс http.ResponseWriter с буферизацией данных.
//
// Используется в middleware HashResponse для перехвата тела ответа
// перед вычислением HMAC-SHA256 хеша.
type hashResponseWriter struct {
	http.ResponseWriter
	key    string
	buffer bytes.Buffer
}

// Write перехватывает запись данных ответа.
// Сохраняет данные в буфер для последующего вычисления хеша,
// а также передает их в оригинальный ResponseWriter.
func (hw *hashResponseWriter) Write(b []byte) (int, error) {
	// Сохраняем данные для вычисления хеша
	hw.buffer.Write(b)
	return hw.ResponseWriter.Write(b)
}

// WriteHeader перехватывает отправку заголовков ответа.
// Вычисляет хеш от буферизованных данных перед отправкой заголовков клиенту.
// Важно: хеш должен быть вычислен до отправки заголовков, так как
// заголовок HashSHA256 должен быть включен в HTTP ответ.
func (hw *hashResponseWriter) WriteHeader(statusCode int) {
	// Вычисляем хеш перед отправкой заголовков
	if hw.buffer.Len() > 0 {
		hashValue := hash.ComputeHMACSHA256(hw.buffer.Bytes(), hw.key)
		if hashValue != "" {
			hw.ResponseWriter.Header().Set("HashSHA256", hashValue)
		}
	}
	hw.ResponseWriter.WriteHeader(statusCode)
}
