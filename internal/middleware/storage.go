package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"go.uber.org/zap"
)

// WithSyncSave добавляет middleware для синхронного сохранения после каждого запроса на обновление
func WithSyncSave(storage storage.Storage, filename string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пропускаем запрос через следующий обработчик
			next.ServeHTTP(w, r)

			// Сохраняем метрики только после запросов на обновление
			if r.Method == http.MethodPost && (strings.HasPrefix(r.URL.Path, "/update") || r.URL.Path == "/update/") {
				ctx := context.Background()
				if err := storage.SaveToFile(ctx, filename); err != nil {
					logger.Log.Error("Failed to sync save metrics",
						zap.String("file", filename),
						zap.Error(err),
					)
				}
			}
		})
	}
}
