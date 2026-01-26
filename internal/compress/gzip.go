package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"sync"
)

// Pools для уменьшения аллокаций
var (
	gzipWriterPool = sync.Pool{
		New: func() interface{} {
			// Используем BestSpeed для производительности
			w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
			return w
		},
	}

	gzipReaderPool = sync.Pool{
		New: func() interface{} {
			return new(gzip.Reader)
		},
	}

	bytesBufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

// GzipCompress сжимает данные в gzip
func GzipCompress(data []byte) ([]byte, error) {
	// Берем буфер из пула
	buf := bytesBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bytesBufferPool.Put(buf)

	// Берем writer из пула
	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(buf)

	if _, err := gw.Write(data); err != nil {
		// Возвращаем writer в пул в случае ошибки
		gzipWriterPool.Put(gw)
		return nil, err
	}

	if err := gw.Close(); err != nil {
		// Возвращаем writer в пул в случае ошибки
		gzipWriterPool.Put(gw)
		return nil, err
	}

	// Возвращаем writer в пул
	gzipWriterPool.Put(gw)

	// Возвращаем копию данных, так как буфер будет переиспользован
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

// compressWriter реализует интерфейс http.ResponseWriter и позволяет прозрачно для сервера
// сжимать передаваемые данные и выставлять правильные HTTP-заголовки
type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer

	wroteHeader bool
}

func NewCompressWriter(w http.ResponseWriter) *compressWriter {
	// Берем writer из пула
	zw := gzipWriterPool.Get().(*gzip.Writer)
	zw.Reset(w)

	return &compressWriter{
		w:  w,
		zw: zw,
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	// Если заголовок еще не отправлен, отправляем его с кодом 200
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if !c.wroteHeader {
		// Устанавливаем Content-Encoding ДО вызова WriteHeader у оригинального writer
		c.w.Header().Set("Content-Encoding", "gzip")
		c.w.WriteHeader(statusCode)
		c.wroteHeader = true
	}
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	err := c.zw.Close()
	// Возвращаем writer в пул
	c.zw.Reset(nil)
	gzipWriterPool.Put(c.zw)
	return err
}

// compressReader реализует интерфейс io.ReadCloser и позволяет прозрачно для сервера
// декомпрессировать получаемые от клиента данные
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func NewCompressReader(r io.ReadCloser) (*compressReader, error) {
	// Берем reader из пула
	zr := gzipReaderPool.Get().(*gzip.Reader)

	if err := zr.Reset(r); err != nil {
		// Если ошибка, возвращаем reader в пул
		gzipReaderPool.Put(zr)
		return nil, err
	}

	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c *compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	// Возвращаем reader в пул перед закрытием оригинального
	err1 := c.zr.Close()
	gzipReaderPool.Put(c.zr)

	// Закрываем оригинальный reader
	err2 := c.r.Close()

	if err1 != nil {
		return err1
	}
	return err2
}
