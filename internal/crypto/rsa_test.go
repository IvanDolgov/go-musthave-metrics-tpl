package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
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
			if string(decrypted) != string(tt.plaintext) {
				t.Errorf("decrypted text mismatch: got %s, want %s", decrypted, tt.plaintext)
			}
		})
	}
}
