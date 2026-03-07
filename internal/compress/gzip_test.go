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

			// Для маленьких данных сжатие может не уменьшить размер
			if len(tt.data) > 100 {
				assert.True(t, len(compressed) <= len(tt.data) || len(tt.data) == 0,
					"Compressed data should be smaller or equal (got %d, original %d)",
					len(compressed), len(tt.data))
			}

			// Для пустых или маленьких данных возвращаем оригинал
			if len(tt.data) == 0 || len(tt.data) < 100 {
				assert.Equal(t, tt.data, compressed)
			} else {
				// Декомпрессируем и проверяем что получили исходные данные
				decompressed, err := decompressForTest(compressed)
				require.NoError(t, err)
				assert.Equal(t, tt.data, decompressed)
			}
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
		require.NotNil(t, cw)
		defer cw.Close()

		// Пишем данные
		data := []byte("test response data")
		n, err := cw.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)

		// Закрываем writer для флаша данных
		err = cw.Close()
		assert.NoError(t, err)

		// Проверяем что заголовки установлены
		assert.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))

		// Проверяем что данные сжаты
		responseBody := recorder.Body.Bytes()
		if len(responseBody) > 0 {
			decompressed, err := decompressForTest(responseBody)
			assert.NoError(t, err)
			assert.Equal(t, data, decompressed)
		}
	})

	t.Run("write header manually", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		require.NotNil(t, cw)
		defer cw.Close()

		// Вручную устанавливаем статус код
		cw.WriteHeader(http.StatusCreated)

		data := []byte("created resource")
		_, err := cw.Write(data)
		assert.NoError(t, err)

		// Проверяем что заголовки установлены
		assert.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
		assert.Equal(t, http.StatusCreated, recorder.Code)
	})

	t.Run("multiple writes", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		require.NotNil(t, cw)
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
		responseBody := recorder.Body.Bytes()
		if len(responseBody) > 0 {
			decompressed, err := decompressForTest(responseBody)
			assert.NoError(t, err)
			assert.Equal(t, expected, decompressed)
		}
	})

	t.Run("write empty data", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		require.NotNil(t, cw)
		defer cw.Close()

		n, err := cw.Write([]byte{})
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("close without write", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		cw := NewCompressWriter(recorder)
		require.NotNil(t, cw)

		err := cw.Close()
		assert.NoError(t, err)

		// Не должно быть данных
		assert.Empty(t, recorder.Body.Bytes())
	})
}

// TestCompressWriterWithNilResponseWriter тестирует создание writer с nil
func TestCompressWriterWithNilResponseWriter(t *testing.T) {
	// Проверяем что не паникует
	cw := NewCompressWriter(nil)
	assert.Nil(t, cw)
}

// TestCompressReader тестирует reader для декомпрессии HTTP запросов
func TestCompressReader(t *testing.T) {
	t.Run("valid gzip data", func(t *testing.T) {
		originalData := []byte("test request data")

		// Сжимаем данные
		compressed, err := GzipCompress(originalData)
		require.NoError(t, err)

		// Для маленьких данных сжатие может вернуть оригинал
		if bytes.Equal(compressed, originalData) {
			t.Skip("Data too small for compression")
		}

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

	t.Run("nil reader", func(t *testing.T) {
		_, err := NewCompressReader(nil)
		assert.Error(t, err)
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
