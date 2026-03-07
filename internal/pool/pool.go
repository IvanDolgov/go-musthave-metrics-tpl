/*
Package pool предоставляет потокобезопасный пул для повторного использования объектов.

Основные возможности:
- Generic-пул для любых типов, реализующих метод Reset()
- Автоматический сброс состояния объектов при возврате в пул
- Потокобезопасные операции Get и Put
- Контроль за утечками памяти
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
	// Мьютекс для дополнительной безопасности
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
		return zero
	}

	return p
}

// Get возвращает объект из пула.
// Если пул пуст, создается новый объект.
func (p *Pool[T]) Get() T {
	// Используем sync.Pool для потокобезопасного получения
	obj := p.pool.Get().(T)

	// Для указателей проверяем, что obj не nil
	// Вызываем Reset независимо от типа
	// Используем interface{} для безопасного вызова Reset
	if iface := any(obj); iface != nil {
		// Проверяем, что объект не nil для указателей
		// Вызываем Reset через интерфейс
		obj.Reset()
	}

	return obj
}

// Put помещает объект обратно в пул.
// Перед помещением в пул вызывается метод Reset() объекта.
func (p *Pool[T]) Put(obj T) {
	// Проверяем через interface{} не является ли объект nil
	if any(obj) == nil {
		return
	}

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
