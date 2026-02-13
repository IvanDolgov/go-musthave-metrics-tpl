package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGzipCompress тестирует сжатие данных
func TestGzipCompress(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "small text",
			data:    []byte("Hello, World!"),
			wantErr: false,
		},
		{
			name:    "json data",
			data:    []byte(`{"key": "value", "number": 123, "array": [1,2,3]}`),
			wantErr: false,
		},
		{
			name:    "repetitive data - good for compression",
			data:    bytes.Repeat([]byte("abc"), 1000),
			wantErr: false,
		},
		{
			name:    "random bytes",
			data:    bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 500),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Сжимаем данные
			compressed, err := GzipCompress(tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, compressed)

			// Проверяем что данные действительно сжаты
			assert.True(t, len(compressed) <= len(tt.data) || len(tt.data) == 0,
				"Compressed data should be smaller or equal (got %d, original %d)",
				len(compressed), len(tt.data))

			// Декомпрессируем и проверяем что получили исходные данные
			decompressed, err := decompressForTest(compressed)
			require.NoError(t, err)
			assert.Equal(t, tt.data, decompressed)
		})
	}
}

// TestGzipCompressConcurrent тестирует конкурентное сжатие
func TestGzipCompressConcurrent(t *testing.T) {
	data := bytes.Repeat([]byte("concurrent test"), 100)

	t.Run("parallel compression", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 50; i++ {
			t.Run("compression", func(t *testing.T) {
				t.Parallel()

				compressed, err := GzipCompress(data)
				assert.NoError(t, err)
				assert.NotEmpty(t, compressed)

				decompressed, err := decompressForTest(compressed)
				assert.NoError(t, err)
				assert.Equal(t, data, decompressed)
			})
		}
	})
}

// TestCompressWriter тестирует writer для сжатия HTTP ответов
func TestCompressWriter(t *testing.T) {
	t.Run("write data with automatic header", func(t *testing.T) {
		// Создаем тестовый ResponseRecorder
		recorder := httptest.NewRecorder()

		// Создаем compressWriter
		cw := NewCompressWriter(recorder)
		defer cw.Close()

		// Пишем данные
		data := []byte("test response data")
		n, err := cw.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)

		// Проверяем что заголовки установлены
		assert.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))

		// Закрываем writer для флаша данных
		err = cw.Close()
		assert.NoError(t, err)

		// Проверяем что данные сжаты
		responseBody := recorder.Body.Bytes()
		decompressed, err := decompressForTest(responseBody)
		assert.NoError(t, err)
		assert.Equal(t, data, decompressed)
	})

	t.Run("write header manually", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		defer cw.Close()

		// Вручную устанавливаем статус код
		cw.WriteHeader(http.StatusCreated)

		// Проверяем что заголовки установлены до записи данных
		assert.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
		assert.Equal(t, http.StatusCreated, recorder.Code)

		data := []byte("created resource")
		_, err := cw.Write(data)
		assert.NoError(t, err)
	})

	t.Run("multiple writes", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		defer cw.Close()

		// Пишем данные частями
		parts := [][]byte{
			[]byte("part1 "),
			[]byte("part2 "),
			[]byte("part3"),
		}

		for _, part := range parts {
			_, err := cw.Write(part)
			assert.NoError(t, err)
		}

		err := cw.Close()
		assert.NoError(t, err)

		// Проверяем что все данные объединены и сжаты
		expected := []byte("part1 part2 part3")
		decompressed, err := decompressForTest(recorder.Body.Bytes())
		assert.NoError(t, err)
		assert.Equal(t, expected, decompressed)
	})

	t.Run("header already sent", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		defer cw.Close()

		// Пишем данные - автоматически отправляет заголовок
		cw.Write([]byte("data"))

		// Пытаемся изменить заголовок после записи
		oldHeader := recorder.Header().Get("Content-Encoding")
		cw.Header().Set("Content-Encoding", "identity")

		// Заголовок не должен измениться
		assert.Equal(t, oldHeader, recorder.Header().Get("Content-Encoding"))
	})
}

// TestCompressWriterWithNilResponseWriter тестирует создание writer с nil
func TestCompressWriterWithNilResponseWriter(t *testing.T) {
	// Просто проверяем что не паникует
	assert.NotPanics(t, func() {
		cw := NewCompressWriter(nil)
		if cw != nil {
			cw.Close()
		}
	})
}

// TestCompressReader тестирует reader для декомпрессии HTTP запросов
func TestCompressReader(t *testing.T) {
	t.Run("valid gzip data", func(t *testing.T) {
		originalData := []byte("test request data")

		// Сжимаем данные
		compressed, err := GzipCompress(originalData)
		require.NoError(t, err)

		// Создаем reader из сжатых данных
		rc := io.NopCloser(bytes.NewReader(compressed))
		cr, err := NewCompressReader(rc)
		require.NoError(t, err)
		defer cr.Close()

		// Читаем декомпрессированные данные
		decompressed, err := io.ReadAll(cr)
		assert.NoError(t, err)
		assert.Equal(t, originalData, decompressed)
	})

	t.Run("invalid gzip data", func(t *testing.T) {
		// Создаем reader с некорректными gzip данными
		invalidData := []byte("not gzip data")
		rc := io.NopCloser(bytes.NewReader(invalidData))

		_, err := NewCompressReader(rc)
		assert.Error(t, err)
	})

	t.Run("empty reader", func(t *testing.T) {
		rc := io.NopCloser(bytes.NewReader([]byte{}))

		_, err := NewCompressReader(rc)
		assert.Error(t, err) // Должно быть ошибка, так как пустые данные не являются корректным gzip
	})

	t.Run("read in chunks", func(t *testing.T) {
		originalData := bytes.Repeat([]byte("chunked data "), 100)

		compressed, err := GzipCompress(originalData)
		require.NoError(t, err)

		rc := io.NopCloser(bytes.NewReader(compressed))
		cr, err := NewCompressReader(rc)
		require.NoError(t, err)
		defer cr.Close()

		// Читаем маленькими кусочками
		buf := make([]byte, 16)
		var result []byte

		for {
			n, err := cr.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}

		assert.Equal(t, originalData, result)
	})
}

// TestCompressReaderClose тестирует закрытие reader
func TestCompressReaderClose(t *testing.T) {
	originalData := []byte("test data")
	compressed, err := GzipCompress(originalData)
	require.NoError(t, err)

	rc := io.NopCloser(bytes.NewReader(compressed))
	cr, err := NewCompressReader(rc)
	require.NoError(t, err)

	// Закрываем reader
	err = cr.Close()
	assert.NoError(t, err)

	// Повторное закрытие не должно паниковать
	assert.NotPanics(t, func() {
		cr.Close()
	})

	// Чтение после закрытия должно вернуть ошибку
	_, err = cr.Read([]byte{})
	assert.Error(t, err)
}

// TestPools тестирует работу пулов объектов
func TestPools(t *testing.T) {
	t.Run("gzip writer pool", func(t *testing.T) {
		// Получаем writer из пула
		w1 := gzipWriterPool.Get().(*gzip.Writer)
		w1.Reset(io.Discard)
		w1.Write([]byte("test"))
		w1.Close()

		// Возвращаем в пул
		w1.Reset(nil)
		gzipWriterPool.Put(w1)

		// Получаем writer снова - должен быть тот же объект
		w2 := gzipWriterPool.Get().(*gzip.Writer)
		assert.NotNil(t, w2)

		// Очищаем
		w2.Reset(nil)
		gzipWriterPool.Put(w2)
	})

	t.Run("gzip reader pool", func(t *testing.T) {
		// Получаем reader из пула
		r1 := gzipReaderPool.Get().(*gzip.Reader)

		// Возвращаем в пул
		gzipReaderPool.Put(r1)

		// Получаем reader снова
		r2 := gzipReaderPool.Get().(*gzip.Reader)
		assert.NotNil(t, r2)
		assert.Equal(t, r1, r2) // Должны получить тот же объект

		gzipReaderPool.Put(r2)
	})

	t.Run("bytes buffer pool", func(t *testing.T) {
		// Получаем буфер из пула
		b1 := bytesBufferPool.Get().(*bytes.Buffer)
		b1.WriteString("test data")

		// Возвращаем в пул
		b1.Reset()
		bytesBufferPool.Put(b1)

		// Получаем буфер снова
		b2 := bytesBufferPool.Get().(*bytes.Buffer)
		assert.NotNil(t, b2)
		assert.Equal(t, 0, b2.Len()) // Должен быть очищен

		bytesBufferPool.Put(b2)
	})
}

// TestIntegrationHTTP тестирует интеграцию с HTTP
func TestIntegrationHTTP(t *testing.T) {
	t.Run("compress response", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cw := NewCompressWriter(w)
			defer cw.Close()

			cw.Write([]byte("compressed response"))
		})

		req := httptest.NewRequest("GET", "http://example.com", nil)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, req)

		// Проверяем заголовки
		assert.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))

		// Проверяем что тело сжато
		body := recorder.Body.Bytes()
		decompressed, err := decompressForTest(body)
		assert.NoError(t, err)
		assert.Equal(t, "compressed response", string(decompressed))
	})

	t.Run("compress request", func(t *testing.T) {
		// Создаем сжатое тело запроса
		originalData := []byte("compressed request body")
		compressed, err := GzipCompress(originalData)
		require.NoError(t, err)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Декомпрессируем тело запроса
			cr, err := NewCompressReader(r.Body)
			require.NoError(t, err)
			defer cr.Close()

			body, err := io.ReadAll(cr)
			assert.NoError(t, err)
			assert.Equal(t, originalData, body)

			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("POST", "http://example.com",
			bytes.NewReader(compressed))
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}

// Benchmark тесты производительности
func BenchmarkGzipCompress(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark test data"), 100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compressed, err := GzipCompress(data)
		if err != nil {
			b.Fatal(err)
		}
		_ = compressed
	}
}

func BenchmarkCompressWriter(b *testing.B) {
	data := []byte("benchmark write data")
	recorder := httptest.NewRecorder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cw := NewCompressWriter(recorder)
		cw.Write(data)
		cw.Close()
	}
}

func BenchmarkCompressReader(b *testing.B) {
	originalData := []byte("benchmark read data")
	compressed, _ := GzipCompress(originalData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc := io.NopCloser(bytes.NewReader(compressed))
		cr, _ := NewCompressReader(rc)
		io.ReadAll(cr)
		cr.Close()
	}
}

func BenchmarkParallelCompress(b *testing.B) {
	data := bytes.Repeat([]byte("parallel compression"), 50)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			compressed, err := GzipCompress(data)
			if err != nil {
				b.Fatal(err)
			}
			_ = compressed
		}
	})
}

// Вспомогательная функция для декомпрессии в тестах
func decompressForTest(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// TestEdgeCases тестирует граничные случаи
func TestEdgeCases(t *testing.T) {
	t.Run("compress very large data", func(t *testing.T) {
		largeData := bytes.Repeat([]byte("large"), 100000)
		compressed, err := GzipCompress(largeData)
		assert.NoError(t, err)
		assert.NotEmpty(t, compressed)

		decompressed, err := decompressForTest(compressed)
		assert.NoError(t, err)
		assert.Equal(t, largeData, decompressed)
	})

	t.Run("writer close without write", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		err := cw.Close()
		assert.NoError(t, err)

		// Не должно быть данных
		assert.Empty(t, recorder.Body.Bytes())
	})

	t.Run("reader with incomplete gzip data", func(t *testing.T) {
		// Создаем неполные gzip данные
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write([]byte("partial"))
		gw.Close()

		partialData := buf.Bytes()[:10] // Берем только часть

		rc := io.NopCloser(bytes.NewReader(partialData))
		_, err := NewCompressReader(rc)
		if err == nil {
			// Может быть успешно, если заголовок прочитался
			// Проверяем что последующее чтение вернет ошибку
			cr, _ := NewCompressReader(rc)
			_, readErr := io.ReadAll(cr)
			assert.Error(t, readErr)
		}
	})
}
