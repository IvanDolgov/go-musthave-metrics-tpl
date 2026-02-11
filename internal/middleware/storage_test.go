package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestWithSyncSaveNilStorage(t *testing.T) {
	logger.Log = zap.NewNop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Передаем nil storage
	middleware := WithSyncSave(nil, "/tmp/test.json")
	wrapped := middleware(handler)

	req := httptest.NewRequest("POST", "/update/gauge/test/42.5", nil)
	rr := httptest.NewRecorder()

	// Не должно паниковать
	assert.NotPanics(t, func() {
		wrapped.ServeHTTP(rr, req)
	})
}
