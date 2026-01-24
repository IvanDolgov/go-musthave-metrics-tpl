package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// RemoteObserver наблюдатель для отправки аудита на удаленный сервер
type RemoteObserver struct {
	url    string
	client *http.Client
}

// NewRemoteObserver создает нового удаленного наблюдателя
func NewRemoteObserver(url string) *RemoteObserver {
	return &RemoteObserver{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: false,
			},
		},
	}
}

// Update отправляет событие на удаленный сервер
func (r *RemoteObserver) Update(event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		logger.Log.Error("Failed to marshal audit event for remote", zap.Error(err))
		return err
	}

	req, err := http.NewRequest("POST", r.url, bytes.NewBuffer(data))
	if err != nil {
		logger.Log.Error("Failed to create audit request",
			zap.String("url", r.url),
			zap.Error(err))
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		logger.Log.Error("Failed to send audit event",
			zap.String("url", r.url),
			zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Log.Error("Audit server returned error",
			zap.String("url", r.url),
			zap.Int("status_code", resp.StatusCode))
		return err
	}

	return nil
}
