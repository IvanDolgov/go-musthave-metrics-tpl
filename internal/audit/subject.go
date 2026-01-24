package audit

import (
	"sync"
)

// ConcreteSubject реализация издателя
type ConcreteSubject struct {
	observers []Observer
	mu        sync.RWMutex
}

// NewConcreteSubject создает новый экземпляр издателя
func NewConcreteSubject() *ConcreteSubject {
	return &ConcreteSubject{
		observers: make([]Observer, 0),
	}
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, observer := range s.observers {
		go observer.Update(event) // асинхронная отправка
	}
}
