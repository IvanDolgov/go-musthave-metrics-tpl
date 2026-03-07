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
	// Для очень маленьких данных сжатие неэффективно
	if len(data) < 100 {
		return data, nil
	}

	// Берем буфер из пула
	buf := bytesBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bytesBufferPool.Put(buf)

	// Берем writer из пула
	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(buf)
	defer func() {
		gw.Reset(nil)
		gzipWriterPool.Put(gw)
	}()

	if _, err := gw.Write(data); err != nil {
		return nil, err
	}

	if err := gw.Close(); err != nil {
		return nil, err
	}

	// Возвращаем копию данных, так как буфер будет переиспользован
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

// compressWriter реализует интерфейс http.ResponseWriter и позволяет прозрачно для сервера
// сжимать передаваемые данные и выставлять правильные HTTP-заголовки
type compressWriter struct {
	w           http.ResponseWriter
	zw          *gzip.Writer
	wroteHeader bool
	wroteData   bool
	closed      bool
}

func NewCompressWriter(w http.ResponseWriter) *compressWriter {
	if w == nil {
		return nil
	}

	// Берем writer из пула
	zw := gzipWriterPool.Get().(*gzip.Writer)
	zw.Reset(w)

	return &compressWriter{
		w:  w,
		zw: zw,
	}
}

func (c *compressWriter) Header() http.Header {
	if c.w == nil {
		return make(http.Header)
	}
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	if c.w == nil {
		return 0, io.ErrClosedPipe
	}

	// Если заголовок еще не отправлен, отправляем его с кодом 200
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}

	if len(p) == 0 {
		return 0, nil
	}

	c.wroteData = true
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if c.w == nil {
		return
	}

	if !c.wroteHeader {
		// Устанавливаем Content-Encoding ДО вызова WriteHeader у оригинального writer
		c.w.Header().Set("Content-Encoding", "gzip")
		c.w.WriteHeader(statusCode)
		c.wroteHeader = true
	}
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	if c == nil || c.closed {
		return nil
	}

	c.closed = true

	var err error
	if c.zw != nil {
		// Если данные не записывались, не нужно закрывать writer
		if c.wroteData {
			err = c.zw.Close()
		}
		// Возвращаем writer в пул
		c.zw.Reset(nil)
		gzipWriterPool.Put(c.zw)
		c.zw = nil
	}

	return err
}

// compressReader реализует интерфейс io.ReadCloser и позволяет прозрачно для сервера
// декомпрессировать получаемые от клиента данные
type compressReader struct {
	r      io.ReadCloser
	zr     *gzip.Reader
	closed bool
}

func NewCompressReader(r io.ReadCloser) (*compressReader, error) {
	if r == nil {
		return nil, io.ErrClosedPipe
	}

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
	if c == nil || c.closed {
		return 0, io.ErrClosedPipe
	}
	if c.zr == nil {
		return 0, io.ErrClosedPipe
	}
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if c == nil || c.closed {
		return nil
	}

	c.closed = true

	var err1, err2 error

	// Возвращаем reader в пул перед закрытием оригинального
	if c.zr != nil {
		err1 = c.zr.Close()
		gzipReaderPool.Put(c.zr)
		c.zr = nil
	}

	// Закрываем оригинальный reader
	if c.r != nil {
		err2 = c.r.Close()
		c.r = nil
	}

	if err1 != nil {
		return err1
	}
	return err2
}
