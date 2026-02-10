package pool

import (
	"sync"
	"testing"
	"time"
)

// TestStruct тестовая структура для тестов, реализующая Reset()
type TestStruct struct {
	Data []byte
	ID   int
	Used bool
}

func (t *TestStruct) Reset() {
	t.Data = t.Data[:0]
	t.Used = false
}

// TestStructNonPointer тестовая структура не указатель
type TestStructNonPointer struct {
	Value int
}

func (t TestStructNonPointer) Reset() {
	t.Value = 0
}

func TestNewPool(t *testing.T) {
	p := New[*TestStruct]()
	if p == nil {
		t.Fatal("New() вернул nil")
	}
}

func TestPoolGetPut(t *testing.T) {
	p := New[*TestStruct]()

	// Получаем объект из пула
	obj1 := p.Get()
	if obj1 == nil {
		t.Fatal("Get() вернул nil")
	}

	// Используем объект
	obj1.Data = append(obj1.Data, "test"...)
	obj1.ID = 123
	obj1.Used = true

	// Возвращаем в пул
	p.Put(obj1)

	// Снова получаем объект (должен быть тот же или новый)
	obj2 := p.Get()

	// Проверяем, что объект был сброшен
	if len(obj2.Data) != 0 {
		t.Errorf("Объект не был сброшен: Data = %v", obj2.Data)
	}
	if obj2.Used {
		t.Error("Объект не был сброшен: Used = true")
	}
}

func TestPoolConcurrent(t *testing.T) {
	p := New[*TestStruct]()
	var wg sync.WaitGroup

	// Запускаем несколько горутин
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			obj := p.Get()
			obj.ID = id
			obj.Data = append(obj.Data, byte(id))
			obj.Used = true

			// Немного "поработаем" с объектом
			time.Sleep(time.Microsecond)

			p.Put(obj)
		}(i)
	}

	wg.Wait()

	// Проверяем, что пул работает без паник
	t.Log("Concurrent test passed")
}

func TestPoolResetCalled(t *testing.T) {
	p := New[*TestStruct]()

	// Получаем объект и заполняем его
	obj := p.Get()
	obj.Data = []byte("test data")
	obj.ID = 42
	obj.Used = true

	// Возвращаем в пул
	p.Put(obj)

	// Получаем объект снова
	obj2 := p.Get()

	// Проверяем, что объект был сброшен
	if len(obj2.Data) != 0 {
		t.Error("Data не был сброшен после Put()")
	}
	if obj2.Used {
		t.Error("Used не был сброшен после Put()")
	}
	if obj2.ID != 0 {
		t.Error("ID не был сброшен после Put()")
	}
}

func TestPoolMultipleTypes(t *testing.T) {
	// Тестируем с разными типами, реализующими Reset()

	// 1. Указатель на структуру
	p1 := New[*TestStruct]()
	obj1 := p1.Get()
	if obj1 == nil {
		t.Error("Get() вернул nil для *TestStruct")
	}
	obj1.ID = 1
	p1.Put(obj1)

	// 2. Не указатель (значение)
	p2 := New[TestStructNonPointer]()
	obj2 := p2.Get()
	obj2.Value = 2
	p2.Put(obj2)

	// Проверяем, что оба пула работают
	t.Log("Multiple types test passed")
}

// TestPoolInterfaceCompliance проверяет, что пул работает с интерфейсами
func TestPoolInterfaceCompliance(t *testing.T) {
	// Тестируем с интерфейсом
	type Resetter interface {
		Reset()
	}

	p := New[*TestStruct]()
	obj := p.Get()

	// Проверяем, что объект реализует интерфейс
	var _ Resetter = obj

	p.Put(obj)
	t.Log("Interface compliance test passed")
}

func BenchmarkPoolGetPut(b *testing.B) {
	p := New[*TestStruct]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := p.Get()
			obj.ID = 1
			p.Put(obj)
		}
	})
}

func BenchmarkPoolWithoutReset(b *testing.B) {
	var pool sync.Pool
	pool.New = func() interface{} {
		return &TestStruct{}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Get().(*TestStruct)
			obj.ID = 1
			pool.Put(obj)
		}
	})
}

func BenchmarkPoolWithManualReset(b *testing.B) {
	var pool sync.Pool
	pool.New = func() interface{} {
		return &TestStruct{}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Get().(*TestStruct)
			obj.ID = 1
			// Ручной сброс
			obj.Reset()
			pool.Put(obj)
		}
	})
}
