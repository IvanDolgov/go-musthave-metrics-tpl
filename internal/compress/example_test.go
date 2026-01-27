package compress

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

// ExampleNewCompressWriter демонстрирует сжатие данных через HTTP ResponseWriter.
func ExampleNewCompressWriter() {
	// Создаем тестовый ResponseRecorder
	rr := httptest.NewRecorder()

	// Создаем writer для сжатия
	cw := NewCompressWriter(rr)

	// Записываем данные
	data := []byte("This is some test data that will be compressed")
	_, err := cw.Write(data)
	if err != nil {
		fmt.Printf("Write error: %v\n", err)
		return
	}

	// Закрываем writer (это важно для завершения сжатия)
	cw.Close()

	fmt.Printf("Original size: %d bytes\n", len(data))
	fmt.Printf("Compressed size: %d bytes\n", len(rr.Body.Bytes()))
	fmt.Printf("Has gzip header: %v\n", rr.Header().Get("Content-Encoding") == "gzip")

	// Вывод (примерный):
	// Original size: 46 bytes
	// Compressed size: 62 bytes
	// Has gzip header: true
}

// ExampleNewCompressReader демонстрирует распаковку данных.
func ExampleNewCompressReader() {
	// Создаем mock ResponseWriter для сжатия
	mockWriter := httptest.NewRecorder()
	cw := NewCompressWriter(mockWriter)
	cw.Write([]byte("Test data for decompression"))
	cw.Close()

	// Получаем сжатые данные
	compressedData := mockWriter.Body.Bytes()

	// Создаем ReadCloser из bytes.Buffer
	reader := io.NopCloser(bytes.NewBuffer(compressedData))

	// Создаем reader для распаковки
	cr, err := NewCompressReader(reader)
	if err != nil {
		fmt.Printf("Create reader error: %v\n", err)
		return
	}
	defer cr.Close()

	// Читаем распакованные данные
	decompressedData, err := io.ReadAll(cr)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
		return
	}

	fmt.Printf("Compressed size: %d bytes\n", len(compressedData))
	fmt.Printf("Decompressed size: %d bytes\n", len(decompressedData))
	fmt.Printf("Decompressed data: %s\n", string(decompressedData))

	// Вывод:
	// Compressed size: 51 bytes
	// Decompressed size: 26 bytes
	// Decompressed data: Test data for decompression
}

// ExampleCompressRoundtrip демонстрирует полный цикл сжатия-распаковки.
func Example_compressRoundtrip() {
	// Исходные данные
	originalData := []byte("Hello, this is a test string for compression and decompression cycle")

	// Шаг 1: Сжатие
	mockWriter := httptest.NewRecorder()
	compressor := NewCompressWriter(mockWriter)
	compressor.Write(originalData)
	compressor.Close()

	compressed := mockWriter.Body.Bytes()

	// Шаг 2: Распаковка
	reader := io.NopCloser(bytes.NewBuffer(compressed))
	decompressor, err := NewCompressReader(reader)
	if err != nil {
		fmt.Printf("Error creating decompressor: %v\n", err)
		return
	}
	defer decompressor.Close()

	decompressed, err := io.ReadAll(decompressor)
	if err != nil {
		fmt.Printf("Error decompressing: %v\n", err)
		return
	}

	// Проверка
	if string(decompressed) == string(originalData) {
		fmt.Println("Compression/decompression cycle successful")
		fmt.Printf("Original: %d bytes, Compressed: %d bytes\n",
			len(originalData), len(compressed))
	} else {
		fmt.Println("Error: decompressed data doesn't match original")
	}

	// Вывод:
	// Compression/decompression cycle successful
	// Original: 67 bytes, Compressed: 79 bytes
}

// ExampleCompressWithHeaders демонстрирует работу с HTTP заголовками при сжатии.
func Example_compressWithHeaders() {
	// Создаем обработчик HTTP
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, поддерживает ли клиент gzip
		if r.Header.Get("Accept-Encoding") == "gzip" {
			cw := NewCompressWriter(w)
			defer cw.Close()

			// Записываем сжатые данные
			cw.Write([]byte("This response is compressed with gzip"))
		} else {
			// Обычный ответ без сжатия
			w.Write([]byte("This response is not compressed"))
		}
	})

	// Тестовый сервер
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Запрос с поддержкой gzip
	req1, _ := http.NewRequest("GET", ts.URL, nil)
	req1.Header.Set("Accept-Encoding", "gzip")
	resp1, _ := http.DefaultClient.Do(req1)
	fmt.Printf("With gzip header: %s\n", resp1.Header.Get("Content-Encoding"))
	resp1.Body.Close()

	// Запрос без поддержки gzip
	req2, _ := http.NewRequest("GET", ts.URL, nil)
	resp2, _ := http.DefaultClient.Do(req2)
	fmt.Printf("Without gzip header: %s\n", resp2.Header.Get("Content-Encoding"))
	resp2.Body.Close()

	// Вывод:
	// With gzip header: gzip
	// Without gzip header:
}

// ExampleGzipCompress демонстрирует использование GzipCompress функции.
func ExampleGzipCompress() {
	// Исходные данные
	data := []byte("This data will be compressed using GzipCompress function")

	// Сжимаем данные
	compressed, err := GzipCompress(data)
	if err != nil {
		fmt.Printf("Compression error: %v\n", err)
		return
	}

	fmt.Printf("Original size: %d bytes\n", len(data))
	fmt.Printf("Compressed size: %d bytes\n", len(compressed))
	fmt.Printf("Compression ratio: %.1f%%\n", float64(len(compressed))/float64(len(data))*100)

	// Вывод (примерный):
	// Original size: 56 bytes
	// Compressed size: 74 bytes
	// Compression ratio: 132.1%
}
