package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// LoadPublicKey загружает публичный ключ из файла
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing public key")
	}

	var pub interface{}
	pub, err = x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Пробуем распарсить как RSA публичный ключ
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
		return pub.(*rsa.PublicKey), nil
	}

	switch pub := pub.(type) {
	case *rsa.PublicKey:
		return pub, nil
	default:
		return nil, errors.New("key type is not RSA")
	}
}

// LoadPrivateKey загружает приватный ключ из файла
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing private key")
	}

	// Пробуем распарсить как PKCS1
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	// Пробуем распарсить как PKCS8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	switch key := key.(type) {
	case *rsa.PrivateKey:
		return key, nil
	default:
		return nil, errors.New("key type is not RSA")
	}
}

// EncryptWithPublicKey шифрует данные с помощью публичного ключа
func EncryptWithPublicKey(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
	// Используем SHA256 для OAEP
	hash := sha256.New()

	// RSA может зашифровать только данные, размер которых меньше модуля ключа
	// Для больших данных нужно использовать гибридное шифрование,
	// но для наших метрик небольшого размера это подходит
	encryptedBytes, err := rsa.EncryptOAEP(hash, rand.Reader, pub, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}
	return encryptedBytes, nil
}

// DecryptWithPrivateKey расшифровывает данные с помощью приватного ключа
func DecryptWithPrivateKey(ciphertext []byte, priv *rsa.PrivateKey) ([]byte, error) {
	// Используем SHA256 для OAEP
	hash := sha256.New()

	decryptedBytes, err := rsa.DecryptOAEP(hash, rand.Reader, priv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	return decryptedBytes, nil
}

// GenerateKeyPair генерирует пару RSA ключей и сохраняет в файлы
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

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privateFile, privateKeyPEM); err != nil {
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

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	if err := pem.Encode(publicFile, publicKeyPEM); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}
