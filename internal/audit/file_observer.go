package audit

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// FileObserver наблюдатель для записи аудита в файл
type FileObserver struct {
	filename string
	mu       sync.Mutex
}

// NewFileObserver создает нового файлового наблюдателя
func NewFileObserver(filename string) *FileObserver {
	return &FileObserver{
		filename: filename,
	}
}

// Update записывает событие в файл
func (f *FileObserver) Update(event *Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		logger.Log.Error("Failed to marshal audit event", zap.Error(err))
		return err
	}

	// Добавляем новую строку
	data = append(data, '\n')

	file, err := os.OpenFile(f.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Error("Failed to open audit file",
			zap.String("filename", f.filename),
			zap.Error(err))
		return err
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		logger.Log.Error("Failed to write audit event to file",
			zap.String("filename", f.filename),
			zap.Error(err))
		return err
	}

	return nil
}
