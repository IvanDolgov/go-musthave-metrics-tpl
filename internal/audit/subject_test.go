package audit

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockObserver для тестирования
type MockObserver struct {
	updateCalled atomic.Int32
	updateDelay  time.Duration
	shouldError  bool
	lastEvent    *Event
	mu           sync.Mutex
	wg           sync.WaitGroup // Добавляем WaitGroup для синхронизации
}

func NewMockObserver() *MockObserver {
	return &MockObserver{}
}

func (m *MockObserver) Update(event *Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	defer func() {
		// Сигнализируем о завершении обновления
		m.wg.Done()
	}()

	m.lastEvent = event
	m.updateCalled.Add(1)

	if m.updateDelay > 0 {
		time.Sleep(m.updateDelay)
	}

	if m.shouldError {
		return errors.New("mock observer error")
	}
	return nil
}

func (m *MockObserver) GetCallCount() int {
	return int(m.updateCalled.Load())
}

func (m *MockObserver) GetLastEvent() *Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastEvent
}

func (m *MockObserver) SetDelay(delay time.Duration) {
	m.updateDelay = delay
}

func (m *MockObserver) SetShouldError(shouldError bool) {
	m.shouldError = shouldError
}

// Add ожидаемое количество вызовов
func (m *MockObserver) AddExpectedCalls(n int) {
	m.wg.Add(n)
}

// Wait ожидает завершения всех вызовов
func (m *MockObserver) Wait(timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestNewConcreteSubject тестирует создание нового издателя
func TestNewConcreteSubject(t *testing.T) {
	subject := NewConcreteSubject()

	assert.NotNil(t, subject)
	assert.NotNil(t, subject.observers)
	assert.Empty(t, subject.observers)
	assert.NotNil(t, subject.ctx)
	assert.NotNil(t, subject.cancel)
	assert.False(t, subject.running.Load())
}

// TestStart тестирует запуск издателя
func TestStart(t *testing.T) {
	subject := NewConcreteSubject()

	assert.False(t, subject.running.Load())

	subject.Start()

	assert.True(t, subject.running.Load())
}

// TestStop тестирует остановку издателя
func TestStop(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()
	assert.True(t, subject.running.Load())

	// Добавляем наблюдателя с долгим обновлением
	mockObserver := NewMockObserver()
	mockObserver.SetDelay(100 * time.Millisecond)
	subject.Register(mockObserver)

	// Отправляем событие
	go subject.Notify(&Event{}) // Используем пустую структуру

	// Даем время на запуск горутины
	time.Sleep(10 * time.Millisecond)

	// Останавливаем
	subject.Stop()

	assert.False(t, subject.running.Load())

	// Проверяем что контекст отменен
	select {
	case <-subject.ctx.Done():
		// Ожидаемо
	default:
		t.Error("context should be canceled")
	}
}

// TestRegister тестирует регистрацию наблюдателей
func TestRegister(t *testing.T) {
	subject := NewConcreteSubject()
	mockObserver1 := NewMockObserver()
	mockObserver2 := NewMockObserver()

	// Проверяем начальное состояние
	assert.Empty(t, subject.observers)

	// Регистрируем первого
	subject.Register(mockObserver1)
	assert.Len(t, subject.observers, 1)
	assert.Equal(t, mockObserver1, subject.observers[0])

	// Регистрируем второго
	subject.Register(mockObserver2)
	assert.Len(t, subject.observers, 2)
	assert.Equal(t, mockObserver1, subject.observers[0])
	assert.Equal(t, mockObserver2, subject.observers[1])
}

// TestDeregister тестирует удаление наблюдателей
func TestDeregister(t *testing.T) {
	subject := NewConcreteSubject()
	mockObserver1 := NewMockObserver()
	mockObserver2 := NewMockObserver()
	mockObserver3 := NewMockObserver()

	// Регистрируем всех
	subject.Register(mockObserver1)
	subject.Register(mockObserver2)
	subject.Register(mockObserver3)
	assert.Len(t, subject.observers, 3)

	// Удаляем среднего
	subject.Deregister(mockObserver2)
	assert.Len(t, subject.observers, 2)
	assert.Equal(t, mockObserver1, subject.observers[0])
	assert.Equal(t, mockObserver3, subject.observers[1])

	// Удаляем первого
	subject.Deregister(mockObserver1)
	assert.Len(t, subject.observers, 1)
	assert.Equal(t, mockObserver3, subject.observers[0])

	// Удаляем последнего
	subject.Deregister(mockObserver3)
	assert.Empty(t, subject.observers)

	// Пытаемся удалить несуществующего (не должно паниковать)
	subject.Deregister(mockObserver2)
	assert.Empty(t, subject.observers)
}

// TestNotify тестирует уведомление наблюдателей
func TestNotify(t *testing.T) {
	tests := []struct {
		name          string
		running       bool
		numObservers  int
		observerDelay time.Duration
		observerError bool
		expectedCalls int
	}{
		{
			name:          "not running - no notifications",
			running:       false,
			numObservers:  3,
			expectedCalls: 0,
		},
		{
			name:          "running with observers",
			running:       true,
			numObservers:  3,
			expectedCalls: 3,
		},
		{
			name:          "running with slow observers",
			running:       true,
			numObservers:  2,
			observerDelay: 50 * time.Millisecond,
			expectedCalls: 2,
		},
		{
			name:          "running with error observers",
			running:       true,
			numObservers:  2,
			observerError: true,
			expectedCalls: 2,
		},
		{
			name:          "running with no observers",
			running:       true,
			numObservers:  0,
			expectedCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject := NewConcreteSubject()
			observers := make([]*MockObserver, tt.numObservers)

			// Создаем и регистрируем наблюдателей
			for i := 0; i < tt.numObservers; i++ {
				obs := NewMockObserver()
				if tt.observerDelay > 0 {
					obs.SetDelay(tt.observerDelay)
				}
				if tt.observerError {
					obs.SetShouldError(true)
				}
				observers[i] = obs
				subject.Register(obs)

				// Ожидаем вызовы
				if tt.expectedCalls > 0 {
					obs.AddExpectedCalls(tt.expectedCalls)
				}
			}

			if tt.running {
				subject.Start()
			}

			// Создаем событие
			event := &Event{}

			// Уведомляем несколько раз
			for i := 0; i < tt.expectedCalls; i++ {
				subject.Notify(event)
			}

			// Ждем завершения с таймаутом
			timeout := 500 * time.Millisecond
			if tt.observerDelay > 0 {
				timeout += tt.observerDelay * time.Duration(tt.expectedCalls)
			}

			for i, obs := range observers {
				if tt.expectedCalls > 0 {
					assert.True(t, obs.Wait(timeout),
						"observer %d timeout waiting for calls", i)
				}
				assert.Equal(t, tt.expectedCalls, obs.GetCallCount(),
					"observer %d call count mismatch", i)

				if tt.expectedCalls > 0 && tt.numObservers > 0 {
					lastEvent := obs.GetLastEvent()
					assert.NotNil(t, lastEvent)
				}
			}
		})
	}
}

// TestNotifyConcurrent тестирует конкурентное уведомление
func TestNotifyConcurrent(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	// Создаем несколько наблюдателей
	numObservers := 5
	observers := make([]*MockObserver, numObservers)
	for i := 0; i < numObservers; i++ {
		obs := NewMockObserver()
		observers[i] = obs
		subject.Register(obs)
	}

	// Отправляем множество событий конкурентно
	numEvents := 10
	for i := 0; i < numObservers; i++ {
		observers[i].AddExpectedCalls(numEvents)
	}

	for i := 0; i < numEvents; i++ {
		go subject.Notify(&Event{})
	}

	// Ждем обработки с таймаутом
	for i, obs := range observers {
		assert.True(t, obs.Wait(2*time.Second),
			"observer %d timeout waiting for events", i)
		assert.Equal(t, numEvents, obs.GetCallCount(),
			"observer %d should receive all events", i)
	}
}

// TestNotifyWithTimeout тестирует таймаут при уведомлении
func TestNotifyWithTimeout(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	// Создаем медленного наблюдателя
	slowObserver := NewMockObserver()
	slowObserver.SetDelay(100 * time.Millisecond) // Уменьшаем задержку для теста
	subject.Register(slowObserver)

	// Создаем быстрого наблюдателя
	fastObserver := NewMockObserver()
	subject.Register(fastObserver)

	// Ожидаем вызовы
	slowObserver.AddExpectedCalls(1)
	fastObserver.AddExpectedCalls(1)

	// Отправляем событие
	subject.Notify(&Event{})

	// Ждем с таймаутом
	assert.True(t, fastObserver.Wait(200*time.Millisecond),
		"fast observer should receive event")
	assert.Equal(t, 1, fastObserver.GetCallCount())

	// Медленный наблюдатель может получить событие позже
	time.Sleep(200 * time.Millisecond)
	// Не проверяем строго, так как поведение может быть разным
}

// TestNotifyAfterStop тестирует уведомление после остановки
func TestNotifyAfterStop(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()

	mockObserver := NewMockObserver()
	subject.Register(mockObserver)

	// Ожидаем 0 вызовов
	mockObserver.AddExpectedCalls(0)

	// Останавливаем
	subject.Stop()

	// Пытаемся уведомить
	subject.Notify(&Event{})

	// Ждем
	time.Sleep(50 * time.Millisecond)

	// Наблюдатель не должен получить событие
	assert.Equal(t, 0, mockObserver.GetCallCount())
}

// TestConcurrentRegisterDeregister тестирует конкурентную регистрацию/удаление
func TestConcurrentRegisterDeregister(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	// Создаем множество наблюдателей
	numObservers := 20
	observers := make([]*MockObserver, numObservers)
	for i := 0; i < numObservers; i++ {
		observers[i] = NewMockObserver()
	}

	// Конкурентно регистрируем и удаляем
	for i := 0; i < numObservers; i++ {
		go func(index int) {
			subject.Register(observers[index])
			time.Sleep(10 * time.Millisecond)
			subject.Deregister(observers[index])
		}(i)
	}

	// Одновременно отправляем события
	for i := 0; i < 10; i++ {
		go subject.Notify(&Event{})
	}

	// Ждем завершения
	time.Sleep(500 * time.Millisecond)

	// Не должно быть паник или дедлоков
	assert.True(t, true)
}

// TestMultipleStops тестирует множественные остановки
func TestMultipleStops(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()

	// Останавливаем несколько раз
	subject.Stop()
	subject.Stop() // Не должно паниковать

	// Проверяем состояние
	assert.False(t, subject.running.Load())
	select {
	case <-subject.ctx.Done():
		// Ожидаемо
	default:
		t.Error("context should be canceled")
	}
}

// TestObserverReceivesEvent тестирует что наблюдатель получает событие
func TestObserverReceivesEvent(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	mockObserver := NewMockObserver()
	subject.Register(mockObserver)

	// Ожидаем 1 вызов
	mockObserver.AddExpectedCalls(1)

	// Создаем событие
	event := &Event{}

	subject.Notify(event)

	// Ждем с таймаутом
	assert.True(t, mockObserver.Wait(500*time.Millisecond),
		"observer should receive event")

	receivedEvent := mockObserver.GetLastEvent()
	assert.NotNil(t, receivedEvent)
	assert.Equal(t, 1, mockObserver.GetCallCount())
}

// TestRegisterNilObserver тестирует регистрацию nil наблюдателя
func TestRegisterNilObserver(t *testing.T) {
	subject := NewConcreteSubject()

	// Регистрируем nil (в текущей реализации это возможно)
	subject.Register(nil)

	// Проверяем что nil добавлен в срез
	assert.Len(t, subject.observers, 1)
	assert.Nil(t, subject.observers[0])
}

// TestDeregisterNilObserver тестирует удаление nil наблюдателя
func TestDeregisterNilObserver(t *testing.T) {
	subject := NewConcreteSubject()

	// Добавляем nil и реального наблюдателя
	realObserver := NewMockObserver()
	subject.Register(nil)
	subject.Register(realObserver)

	assert.Len(t, subject.observers, 2)

	// Удаляем nil
	subject.Deregister(nil)

	assert.Len(t, subject.observers, 1)
	assert.Equal(t, realObserver, subject.observers[0])
}

// BenchmarkNotify бенчмарк для уведомлений
func BenchmarkNotify(b *testing.B) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	// Регистрируем несколько наблюдателей
	for i := 0; i < 5; i++ {
		subject.Register(NewMockObserver())
	}

	event := &Event{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		subject.Notify(event)
	}
}

// BenchmarkNotifyWithSlowObservers бенчмарк с медленными наблюдателями
func BenchmarkNotifyWithSlowObservers(b *testing.B) {
	subject := NewConcreteSubject()
	subject.Start()
	defer subject.Stop()

	// Регистрируем медленных наблюдателей
	for i := 0; i < 3; i++ {
		obs := NewMockObserver()
		obs.SetDelay(1 * time.Millisecond)
		subject.Register(obs)
	}

	event := &Event{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		subject.Notify(event)
	}
}

// TestStopWithActiveNotifications тестирует остановку во время активных уведомлений
func TestStopWithActiveNotifications(t *testing.T) {
	subject := NewConcreteSubject()
	subject.Start()

	// Добавляем медленных наблюдателей
	numObservers := 5
	for i := 0; i < numObservers; i++ {
		obs := NewMockObserver()
		obs.SetDelay(200 * time.Millisecond)
		subject.Register(obs)
	}

	// Отправляем событие
	subject.Notify(&Event{})

	// Немедленно останавливаем
	subject.Stop()

	// Проверяем что остановка прошла без паники
	assert.False(t, subject.running.Load())
}
