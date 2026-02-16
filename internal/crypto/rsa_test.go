package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateKeyPair проверяет генерацию пары ключей
func TestGenerateKeyPair(t *testing.T) {
	// Создаем временную директорию для ключей
	tempDir := t.TempDir()
	privateKeyPath := filepath.Join(tempDir, "private.pem")
	publicKeyPath := filepath.Join(tempDir, "public.pem")

	// Генерируем ключи (используем меньший размер для тестов)
	err := GenerateKeyPair(privateKeyPath, publicKeyPath, 2048)
	require.NoError(t, err, "GenerateKeyPair should not return error")

	// Проверяем, что файлы созданы
	_, err = os.Stat(privateKeyPath)
	assert.NoError(t, err, "Private key file should exist")

	_, err = os.Stat(publicKeyPath)
	assert.NoError(t, err, "Public key file should exist")

	// Проверяем, что файлы не пустые
	privateInfo, _ := os.Stat(privateKeyPath)
	assert.Greater(t, privateInfo.Size(), int64(0), "Private key file should not be empty")

	publicInfo, _ := os.Stat(publicKeyPath)
	assert.Greater(t, publicInfo.Size(), int64(0), "Public key file should not be empty")
}

// TestLoadPublicKey проверяет загрузку публичного ключа
func TestLoadPublicKey(t *testing.T) {
	// Создаем временную директорию
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")

	// Генерируем ключи
	err := GenerateKeyPair(filepath.Join(tempDir, "private.pem"), publicKeyPath, 2048)
	require.NoError(t, err)

	// Загружаем публичный ключ
	pubKey, err := LoadPublicKey(publicKeyPath)
	require.NoError(t, err, "LoadPublicKey should not return error")
	assert.NotNil(t, pubKey, "Public key should not be nil")
	assert.IsType(t, &rsa.PublicKey{}, pubKey, "Should return *rsa.PublicKey")

	// Проверяем, что ключ рабочий (может шифровать)
	testMsg := []byte("test message")
	_, err = rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, testMsg, nil)
	assert.NoError(t, err, "Public key should be usable for encryption")
}

// TestLoadPrivateKey проверяет загрузку приватного ключа
func TestLoadPrivateKey(t *testing.T) {
	// Создаем временную директорию
	tempDir := t.TempDir()
	privateKeyPath := filepath.Join(tempDir, "private.pem")

	// Генерируем ключи
	err := GenerateKeyPair(privateKeyPath, filepath.Join(tempDir, "public.pem"), 2048)
	require.NoError(t, err)

	// Загружаем приватный ключ
	privKey, err := LoadPrivateKey(privateKeyPath)
	require.NoError(t, err, "LoadPrivateKey should not return error")
	assert.NotNil(t, privKey, "Private key should not be nil")
	assert.IsType(t, &rsa.PrivateKey{}, privKey, "Should return *rsa.PrivateKey")
}

// TestEncryptDecrypt проверяет шифрование и расшифровку
func TestEncryptDecrypt(t *testing.T) {
	// Генерируем ключи в памяти
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	testCases := []struct {
		name    string
		message []byte
	}{
		{
			name:    "short message",
			message: []byte("hello"),
		},
		{
			name:    "longer message",
			message: []byte("this is a test message for encryption"),
		},
		{
			name:    "empty message",
			message: []byte{},
		},
		{
			name:    "message with special chars",
			message: []byte("!@#$%^&*()_+{}[]|\\:;\"'<>,.?/~`"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Шифруем
			encrypted, err := EncryptWithPublicKey(tc.message, publicKey)
			require.NoError(t, err, "EncryptWithPublicKey should not return error")
			assert.NotEmpty(t, encrypted, "Encrypted data should not be empty")
			assert.NotEqual(t, tc.message, encrypted, "Encrypted data should differ from original")

			// Расшифровываем
			decrypted, err := DecryptWithPrivateKey(encrypted, privateKey)
			require.NoError(t, err, "DecryptWithPrivateKey should not return error")

			// Проверяем, что расшифрованное сообщение совпадает с оригиналом
			assert.True(t, bytes.Equal(tc.message, decrypted),
				"Decrypted message should match original.\nExpected: %v\nActual: %v",
				tc.message, decrypted)
		})
	}
}

// TestEncryptDecryptWithKeyFiles проверяет полный цикл с файлами ключей
func TestEncryptDecryptWithKeyFiles(t *testing.T) {
	// Создаем временную директорию
	tempDir := t.TempDir()
	privateKeyPath := filepath.Join(tempDir, "private.pem")
	publicKeyPath := filepath.Join(tempDir, "public.pem")

	// Генерируем ключи
	err := GenerateKeyPair(privateKeyPath, publicKeyPath, 2048)
	require.NoError(t, err)

	// Загружаем ключи
	pubKey, err := LoadPublicKey(publicKeyPath)
	require.NoError(t, err)

	privKey, err := LoadPrivateKey(privateKeyPath)
	require.NoError(t, err)

	// Тестовое сообщение
	originalMsg := []byte("secret metric data")

	// Шифруем публичным ключом
	encrypted, err := EncryptWithPublicKey(originalMsg, pubKey)
	require.NoError(t, err)

	// Расшифровываем приватным ключом
	decrypted, err := DecryptWithPrivateKey(encrypted, privKey)
	require.NoError(t, err)

	// Проверяем
	assert.Equal(t, originalMsg, decrypted, "Decrypted message should match original")
}

// TestLoadInvalidKey проверяет обработку ошибок при загрузке ключей
func TestLoadInvalidKey(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("non-existent file", func(t *testing.T) {
		_, err := LoadPublicKey(filepath.Join(tempDir, "nonexistent.pem"))
		assert.Error(t, err, "Should return error for non-existent file")

		_, err = LoadPrivateKey(filepath.Join(tempDir, "nonexistent.pem"))
		assert.Error(t, err, "Should return error for non-existent file")
	})

	t.Run("invalid pem file", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.pem")
		err := os.WriteFile(invalidPath, []byte("not a valid key"), 0644)
		require.NoError(t, err)

		_, err = LoadPublicKey(invalidPath)
		assert.Error(t, err, "Should return error for invalid PEM")

		_, err = LoadPrivateKey(invalidPath)
		assert.Error(t, err, "Should return error for invalid PEM")
	})

	t.Run("wrong key type", func(t *testing.T) {
		// Создаем временный файл с неправильным типом ключа
		wrongPath := filepath.Join(tempDir, "wrong.pem")

		// Генерируем неподходящий PEM блок
		block := &pem.Block{
			Type:  "WRONG TYPE",
			Bytes: []byte("some data"),
		}

		file, err := os.Create(wrongPath)
		require.NoError(t, err)
		err = pem.Encode(file, block)
		file.Close()
		require.NoError(t, err)

		_, err = LoadPublicKey(wrongPath)
		assert.Error(t, err, "Should return error for wrong key type")
	})
}

// TestEncryptWithNilKey проверяет шифрование с nil ключом
func TestEncryptWithNilKey(t *testing.T) {
	msg := []byte("test")
	_, err := EncryptWithPublicKey(msg, nil)
	assert.Error(t, err, "Should return error when public key is nil")
}

// TestDecryptWithNilKey проверяет расшифровку с nil ключом
func TestDecryptWithNilKey(t *testing.T) {
	msg := []byte("test")
	_, err := DecryptWithPrivateKey(msg, nil)
	assert.Error(t, err, "Should return error when private key is nil")
}

// TestEncryptTooLargeData проверяет шифрование слишком больших данных
func TestEncryptTooLargeData(t *testing.T) {
	// Генерируем ключи
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Для RSA-2048 максимальный размер данных для OAEP с SHA-256 примерно 190 байт
	// Создаем данные больше этого размера
	largeData := make([]byte, 300)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	_, err = EncryptWithPublicKey(largeData, &privateKey.PublicKey)
	assert.Error(t, err, "Should return error for data too large")
}

// BenchmarkEncrypt бенчмарк для шифрования
func BenchmarkEncrypt(b *testing.B) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(b, err)

	msg := []byte("benchmark test message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncryptWithPublicKey(msg, &privateKey.PublicKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecrypt бенчмарк для расшифровки
func BenchmarkDecrypt(b *testing.B) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(b, err)

	msg := []byte("benchmark test message")
	encrypted, err := EncryptWithPublicKey(msg, &privateKey.PublicKey)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecryptWithPrivateKey(encrypted, privateKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}
