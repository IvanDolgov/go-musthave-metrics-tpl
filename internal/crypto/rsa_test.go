package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"
)

func TestHybridEncryption(t *testing.T) {
	// Генерируем тестовую пару ключей
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &privateKey.PublicKey

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"small data", []byte("hello")},
		{"medium data", []byte("hello world this is a test message")},
		{"large data", bytes.Repeat([]byte("A"), 1024*10)}, // 10KB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Шифруем
			ciphertext, err := EncryptWithHybrid(tt.plaintext, publicKey)
			if err != nil {
				t.Fatal(err)
			}

			// Расшифровываем
			decrypted, err := DecryptWithHybrid(ciphertext, privateKey)
			if err != nil {
				t.Fatal(err)
			}

			// Проверяем
			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("decrypted text mismatch: got %s, want %s", decrypted, tt.plaintext)
			}
		})
	}
}

func TestDirectRSAEncryption(t *testing.T) {
	// Генерируем тестовую пару ключей
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &privateKey.PublicKey

	// Для прямого RSA шифрования данные должны быть меньше модуля RSA
	// Для 2048 бит максимальный размер: 2048/8 - 2*hashSize - 2 = 256 - 2*32 - 2 = 190 байт
	plaintext := []byte("small data for RSA direct encryption")

	t.Run("direct RSA encryption/decryption", func(t *testing.T) {
		// Шифруем
		ciphertext, err := EncryptWithPublicKey(plaintext, publicKey)
		if err != nil {
			t.Fatal(err)
		}

		// Расшифровываем
		decrypted, err := DecryptWithPrivateKey(ciphertext, privateKey)
		if err != nil {
			t.Fatal(err)
		}

		// Проверяем
		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("decrypted text mismatch: got %s, want %s", decrypted, plaintext)
		}
	})

	t.Run("direct RSA with too large data should fail", func(t *testing.T) {
		// Пытаемся зашифровать слишком большие данные
		largeData := bytes.Repeat([]byte("A"), 200) // 200 байт > 190
		_, err := EncryptWithPublicKey(largeData, publicKey)
		if err == nil {
			t.Error("expected error for too large data, got nil")
		}
	})
}

func TestGenerateAndLoadKeys(t *testing.T) {
	// Создаем временную директорию для ключей
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "private.pem")
	publicKeyPath := filepath.Join(tmpDir, "public.pem")

	t.Run("generate and load keys", func(t *testing.T) {
		// Генерируем ключи
		err := GenerateKeyPair(privateKeyPath, publicKeyPath, 2048)
		if err != nil {
			t.Fatal(err)
		}

		// Проверяем, что файлы созданы
		if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
			t.Error("private key file not created")
		}
		if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
			t.Error("public key file not created")
		}

		// Загружаем приватный ключ
		privateKey, err := LoadPrivateKey(privateKeyPath)
		if err != nil {
			t.Fatal(err)
		}

		// Загружаем публичный ключ
		publicKey, err := LoadPublicKey(publicKeyPath)
		if err != nil {
			t.Fatal(err)
		}

		// Проверяем, что ключи работают
		message := []byte("test message for generated keys")

		// Пробуем гибридное шифрование
		ciphertext, err := EncryptWithHybrid(message, publicKey)
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := DecryptWithHybrid(ciphertext, privateKey)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(decrypted, message) {
			t.Errorf("decrypted text mismatch: got %s, want %s", decrypted, message)
		}
	})

	t.Run("generate with different key sizes", func(t *testing.T) {
		sizes := []int{1024, 2048, 4096}
		for _, size := range sizes {
			t.Run(string(rune(size)), func(t *testing.T) {
				privatePath := filepath.Join(tmpDir, "private_"+string(rune(size))+".pem")
				publicPath := filepath.Join(tmpDir, "public_"+string(rune(size))+".pem")

				err := GenerateKeyPair(privatePath, publicPath, size)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}

func TestLoadKeysErrors(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("load non-existent private key", func(t *testing.T) {
		_, err := LoadPrivateKey(filepath.Join(tmpDir, "nonexistent.pem"))
		if err == nil {
			t.Error("expected error for non-existent file, got nil")
		}
	})

	t.Run("load non-existent public key", func(t *testing.T) {
		_, err := LoadPublicKey(filepath.Join(tmpDir, "nonexistent.pem"))
		if err == nil {
			t.Error("expected error for non-existent file, got nil")
		}
	})

	t.Run("load invalid key file", func(t *testing.T) {
		// Создаем файл с некорректным содержимым
		invalidPath := filepath.Join(tmpDir, "invalid.pem")
		err := os.WriteFile(invalidPath, []byte("not a valid PEM"), 0644)
		if err != nil {
			t.Fatal(err)
		}

		_, err = LoadPrivateKey(invalidPath)
		if err == nil {
			t.Error("expected error for invalid PEM, got nil")
		}

		_, err = LoadPublicKey(invalidPath)
		if err == nil {
			t.Error("expected error for invalid PEM, got nil")
		}
	})
}

func TestHybridEncryptionErrors(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := &privateKey.PublicKey

	t.Run("decrypt with corrupted data", func(t *testing.T) {
		message := []byte("test message")
		ciphertext, err := EncryptWithHybrid(message, publicKey)
		if err != nil {
			t.Fatal(err)
		}

		// Повреждаем данные
		if len(ciphertext) > 10 {
			ciphertext[10] ^= 0xFF // инвертируем байт
		}

		_, err = DecryptWithHybrid(ciphertext, privateKey)
		if err == nil {
			t.Error("expected error for corrupted data, got nil")
		}
	})

	t.Run("decrypt with too short ciphertext", func(t *testing.T) {
		_, err := DecryptWithHybrid([]byte{1, 2, 3}, privateKey)
		if err == nil {
			t.Error("expected error for too short ciphertext, got nil")
		}
	})

	t.Run("encrypt with nil public key", func(t *testing.T) {
		_, err := EncryptWithHybrid([]byte("test"), nil)
		if err == nil {
			t.Error("expected error for nil public key, got nil")
		}
	})

	t.Run("decrypt with nil private key", func(t *testing.T) {
		_, err := DecryptWithHybrid([]byte{1, 2, 3, 4}, nil)
		if err == nil {
			t.Error("expected error for nil private key, got nil")
		}
	})
}

func TestKeyCompatibility(t *testing.T) {
	// Генерируем две пары ключей
	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("secret message")

	t.Run("encrypt with key1, decrypt with key1 should work", func(t *testing.T) {
		ciphertext, err := EncryptWithHybrid(message, &key1.PublicKey)
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := DecryptWithHybrid(ciphertext, key1)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(decrypted, message) {
			t.Error("decryption failed with correct key")
		}
	})

	t.Run("encrypt with key1, decrypt with key2 should fail", func(t *testing.T) {
		ciphertext, err := EncryptWithHybrid(message, &key1.PublicKey)
		if err != nil {
			t.Fatal(err)
		}

		_, err = DecryptWithHybrid(ciphertext, key2)
		if err == nil {
			t.Error("expected error when decrypting with wrong key, got nil")
		}
	})
}

func BenchmarkHybridEncryption(b *testing.B) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	publicKey := &privateKey.PublicKey

	message := bytes.Repeat([]byte("A"), 1024) // 1KB

	b.Run("encrypt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := EncryptWithHybrid(message, publicKey)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Предварительно шифруем для теста расшифровки
	ciphertext, err := EncryptWithHybrid(message, publicKey)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("decrypt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := DecryptWithHybrid(ciphertext, privateKey)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
