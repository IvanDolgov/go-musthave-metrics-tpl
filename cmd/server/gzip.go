package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// compressWriter реализует интерфейс http.ResponseWriter и позволяет прозрачно для сервера
// сжимать передаваемые данные и выставлять правильные HTTP-заголовки
type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	return &compressWriter{
		w:  w,
		zw: gzip.NewWriter(w),
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	return c.zw.Close()
}

// compressReader реализует интерфейс io.ReadCloser и позволяет прозрачно для сервера
// декомпрессировать получаемые от клиента данные
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		return err
	}
	return c.zr.Close()
}

// smartCompressWriter сжимает только определенные типы контента
type smartCompressWriter struct {
	w              http.ResponseWriter
	zw             *gzip.Writer
	shouldCompress bool
}

func newSmartCompressWriter(w http.ResponseWriter) *smartCompressWriter {
	return &smartCompressWriter{
		w:  w,
		zw: gzip.NewWriter(w),
	}
}

func (c *smartCompressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *smartCompressWriter) Write(p []byte) (int, error) {
	// Проверяем Content-Type ответа чтобы решить, сжимать или нет
	contentType := c.w.Header().Get("Content-Type")
	c.shouldCompress = strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")

	if c.shouldCompress {
		return c.zw.Write(p)
	}
	// Если не нужно сжимать - пишем напрямую
	return c.w.Write(p)
}

func (c *smartCompressWriter) WriteHeader(statusCode int) {
	// Устанавливаем Content-Encoding только если будем сжимать
	contentType := c.w.Header().Get("Content-Type")
	c.shouldCompress = strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")

	if c.shouldCompress && statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

func (c *smartCompressWriter) Close() error {
	if c.shouldCompress {
		return c.zw.Close()
	}
	return nil
}
