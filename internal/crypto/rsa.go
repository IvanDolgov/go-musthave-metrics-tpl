// Package crypto предоставляет функции для шифрования и работы с ключами
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

const (
	// AESKeySize размер ключа AES-256
	AESKeySize = 32
	// NonceSize размер nonce для AES-GCM
	NonceSize = 12
)

// GenerateKeyPair генерирует пару RSA ключей и сохраняет их в файлы
func GenerateKeyPair(privateKeyPath, publicKeyPath string, bits int) error {
	// Генерируем приватный ключ
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Сохраняем приватный ключ
	privateFile, err := os.Create(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer privateFile.Close()

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	if err := pem.Encode(privateFile, privateBlock); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Сохраняем публичный ключ
	publicFile, err := os.Create(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer publicFile.Close()

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	if err := pem.Encode(publicFile, publicBlock); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}

// LoadPrivateKey загружает приватный ключ из файла
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, fmt.Errorf("private key path is empty")
	}

	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Пробуем PKCS8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is not RSA private key")
		}
	}

	return privateKey, nil
}

// LoadPublicKey загружает публичный ключ из файла
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	if path == "" {
		return nil, fmt.Errorf("public key path is empty")
	}

	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return publicKey, nil
}

// EncryptWithPublicKey шифрует данные с использованием RSA (только для малых данных)
// Внимание: этот метод имеет ограничение на размер данных!
// Для больших данных используйте EncryptWithHybrid
func EncryptWithPublicKey(msg []byte, pubKey *rsa.PublicKey) ([]byte, error) {
	if pubKey == nil {
		return nil, fmt.Errorf("public key is nil")
	}
	if len(msg) == 0 {
		return nil, fmt.Errorf("message is empty")
	}
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, msg, nil)
}

// DecryptWithPrivateKey расшифровывает данные с использованием RSA
func DecryptWithPrivateKey(ciphertext []byte, privKey *rsa.PrivateKey) ([]byte, error) {
	if privKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("ciphertext is empty")
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, ciphertext, nil)
}

// EncryptWithHybrid шифрует данные с использованием гибридной схемы:
// 1. Генерируется случайный AES-ключ
// 2. Данные шифруются AES-GCM
// 3. AES-ключ шифруется RSA-OAEP
// Формат: [длина зашифрованного ключа:4][зашифрованный ключ][nonce:12][зашифрованные данные]
func EncryptWithHybrid(plaintext []byte, pubKey *rsa.PublicKey) ([]byte, error) {
	if pubKey == nil {
		return nil, fmt.Errorf("public key is nil")
	}
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("plaintext is empty")
	}

	// 1. Генерируем случайный AES-ключ
	aesKey := make([]byte, AESKeySize)
	if _, err := io.ReadFull(rand.Reader, aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	// 2. Шифруем AES-ключ RSA
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	// 3. Шифруем данные AES-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Генерируем nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Шифруем данные
	encryptedData := aesGCM.Seal(nil, nonce, plaintext, nil)

	// 4. Собираем результат: [длина ключа:4][ключ][nonce:12][данные]
	result := make([]byte, 4+len(encryptedKey)+NonceSize+len(encryptedData))

	// Записываем длину зашифрованного ключа
	binary.BigEndian.PutUint32(result[0:4], uint32(len(encryptedKey)))

	// Записываем зашифрованный ключ
	copy(result[4:4+len(encryptedKey)], encryptedKey)

	// Записываем nonce
	copy(result[4+len(encryptedKey):4+len(encryptedKey)+NonceSize], nonce)

	// Записываем зашифрованные данные
	copy(result[4+len(encryptedKey)+NonceSize:], encryptedData)

	return result, nil
}

// DecryptWithHybrid расшифровывает данные, зашифрованные гибридной схемой
func DecryptWithHybrid(ciphertext []byte, privKey *rsa.PrivateKey) ([]byte, error) {
	if privKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}
	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("ciphertext too short: need at least 4 bytes, got %d", len(ciphertext))
	}

	// 1. Читаем длину зашифрованного ключа
	keyLen := binary.BigEndian.Uint32(ciphertext[0:4])

	if len(ciphertext) < int(4+keyLen+NonceSize) {
		return nil, fmt.Errorf("ciphertext too short for key and nonce: need %d, got %d",
			4+keyLen+NonceSize, len(ciphertext))
	}

	// 2. Извлекаем зашифрованный ключ
	encryptedKey := ciphertext[4 : 4+keyLen]

	// 3. Извлекаем nonce
	nonce := ciphertext[4+keyLen : 4+keyLen+NonceSize]

	// 4. Извлекаем зашифрованные данные
	encryptedData := ciphertext[4+keyLen+NonceSize:]

	// 5. Расшифровываем AES-ключ RSA
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, encryptedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// 6. Расшифровываем данные AES
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := aesGCM.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}
