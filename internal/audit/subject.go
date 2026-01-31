package audit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ConcreteSubject реализация издателя с контролем горутин
type ConcreteSubject struct {
	observers []Observer
	mu        sync.RWMutex
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	running   atomic.Bool
}

// NewConcreteSubject создает новый экземпляр издателя
func NewConcreteSubject() *ConcreteSubject {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConcreteSubject{
		observers: make([]Observer, 0),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start запускает обработку событий
func (s *ConcreteSubject) Start() {
	s.running.Store(true)
}

// Stop останавливает все горутины и ждет их завершения
func (s *ConcreteSubject) Stop() {
	s.running.Store(false)
	s.cancel()
	s.wg.Wait()
}

// Register регистрирует нового наблюдателя
func (s *ConcreteSubject) Register(observer Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, observer)
}

// Deregister удаляет наблюдателя
func (s *ConcreteSubject) Deregister(observer Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, obs := range s.observers {
		if obs == observer {
			s.observers = append(s.observers[:i], s.observers[i+1:]...)
			break
		}
	}
}

// Notify уведомляет всех наблюдателей о событии
func (s *ConcreteSubject) Notify(event *Event) {
	if !s.running.Load() {
		return
	}

	s.mu.RLock()
	observers := make([]Observer, len(s.observers))
	copy(observers, s.observers)
	s.mu.RUnlock()

	for _, observer := range observers {
		s.wg.Add(1)
		go s.notifyObserver(observer, event)
	}
}

// notifyObserver уведомляет одного наблюдателя с контролем горутин
func (s *ConcreteSubject) notifyObserver(observer Observer, event *Event) {
	defer s.wg.Done()

	select {
	case <-s.ctx.Done():
		return
	default:
		// Запускаем с таймаутом для безопасности
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- observer.Update(event)
		}()

		select {
		case <-ctx.Done():
			// Таймаут - логируем но не паникуем
			return
		case err := <-done:
			if err != nil {
				// Логирование ошибки уже в observer.Update
				return
			}
		}
	}
}
