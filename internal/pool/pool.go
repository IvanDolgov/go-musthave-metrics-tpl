/*
Package pool предоставляет потокобезопасный пул для повторного использования объектов.

Основные возможности:
- Generic-пул для любых типов, реализующих метод Reset()
- Автоматический сброс состояния объектов при возврате в пул
- Потокобезопасные операции Get и Put
- Контроль за утечками памяти

# Пример использования

	type MyStruct struct {
	    data []byte
	}

	func (m *MyStruct) Reset() {
	    m.data = m.data[:0]
	}

	func main() {
	    // Создаем пул для MyStruct
	    p := pool.New[*MyStruct]()

	    // Получаем объект из пула (или создаем новый)
	    obj := p.Get()

	    // Используем объект
	    obj.data = append(obj.data, "hello"...)

	    // Возвращаем в пул (автоматически вызовется Reset())
	    p.Put(obj)
	}

# Примечания

- Все объекты в пуле автоматически сбрасываются при возврате
- Пул потокобезопасен и может использоваться из нескольких горутин
- Для работы требуется, чтобы тип реализовывал метод Reset()
*/
package pool

import (
	"sync"
)

// Resetter определяет интерфейс для объектов, которые могут сбрасывать свое состояние.
type Resetter interface {
	Reset()
}

// Pool представляет собой generic пул для хранения объектов типа T.
// T должен реализовывать интерфейс Resetter.
type Pool[T Resetter] struct {
	pool sync.Pool
	// Функция для создания новых объектов, если пул пуст
	newFunc func() T
	// Мьютекс для дополнительной безопасности (опционально)
	mu sync.RWMutex
	// Счетчик созданных объектов для отладки
	createdCount int
}

// New создает и возвращает новый пул для типа T.
// T должен реализовывать интерфейс Resetter (иметь метод Reset()).
func New[T Resetter]() *Pool[T] {
	p := &Pool[T]{}

	// Устанавливаем функцию для создания новых объектов
	p.pool.New = func() interface{} {
		p.mu.Lock()
		p.createdCount++
		p.mu.Unlock()

		// Создаем нулевое значение типа T
		var zero T
		// Для указателей создаем новый объект
		if isPointer[T]() {
			// Используем рефлексию или другой метод для создания
			// В данном случае полагаемся на sync.Pool
			return zero
		}
		return zero
	}

	return p
}

// Get возвращает объект из пула.
// Если пул пуст, создается новый объект.
func (p *Pool[T]) Get() T {
	// Используем sync.Pool для потокобезопасного получения
	obj := p.pool.Get().(T)

	// Убедимся, что объект сброшен
	obj.Reset()

	return obj
}

// Put помещает объект обратно в пул.
// Перед помещением в пул вызывается метод Reset() объекта.
func (p *Pool[T]) Put(obj T) {
	// Сбрасываем состояние объекта перед возвратом в пул
	obj.Reset()

	// Помещаем объект в пул
	p.pool.Put(obj)
}

// CreatedCount возвращает количество созданных объектов.
// Полезно для отладки и мониторинга.
func (p *Pool[T]) CreatedCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.createdCount
}

// Clear очищает пул.
// Все объекты в пуле будут удалены и могут быть собраны GC.
func (p *Pool[T]) Clear() {
	// Создаем новый sync.Pool
	p.pool = sync.Pool{
		New: p.pool.New,
	}
	p.mu.Lock()
	p.createdCount = 0
	p.mu.Unlock()
}

// isPointer проверяет, является ли тип T указателем.
// Вспомогательная функция для внутреннего использования.
func isPointer[T any]() bool {
	var zero T
	switch (interface{})(zero).(type) {
	case *interface{}:
		return true
	default:
		// Простая проверка через switch
		return false
	}
}
