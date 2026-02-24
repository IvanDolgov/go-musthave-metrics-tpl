// Package middleware предоставляет middleware для HTTP сервера
package middleware

import (
	"bytes"
	"compress/gzip"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/crypto"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// DecryptionMiddleware создает middleware для расшифровки тела запроса
func DecryptionMiddleware(privateKeyPath string) func(next http.Handler) http.Handler {
	var privateKey interface{} // *rsa.PrivateKey

	if privateKeyPath != "" {
		key, err := crypto.LoadPrivateKey(privateKeyPath)
		if err != nil {
			logger.Log.Error("Failed to load private key", zap.String("path", privateKeyPath), zap.Error(err))
		} else {
			privateKey = key
			logger.Log.Info("Private key loaded for decryption", zap.String("path", privateKeyPath))
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if privateKey == nil || r.Header.Get("X-Encrypted") != "true" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			var dataToDecrypt []byte
			if r.Header.Get("Content-Encoding") == "gzip" {
				gzipReader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					http.Error(w, "Failed to decompress data", http.StatusBadRequest)
					return
				}
				dataToDecrypt, err = io.ReadAll(gzipReader)
				gzipReader.Close()
				if err != nil {
					http.Error(w, "Failed to read decompressed data", http.StatusBadRequest)
					return
				}
			} else {
				dataToDecrypt = body
			}

			privKey := privateKey.(*rsa.PrivateKey)
			decryptedData, err := crypto.DecryptWithHybrid(dataToDecrypt, privKey)
			if err != nil {
				logger.Log.Error("Failed to decrypt request body", zap.Error(err))
				http.Error(w, "Decryption failed", http.StatusBadRequest)
				return
			}

			logger.Log.Debug("Request decrypted successfully",
				zap.Int("encrypted_size", len(dataToDecrypt)),
				zap.Int("decrypted_size", len(decryptedData)))

			r.Body = io.NopCloser(bytes.NewReader(decryptedData))
			r.ContentLength = int64(len(decryptedData))
			r.Header.Del("X-Encrypted")

			next.ServeHTTP(w, r)
		})
	}
}
