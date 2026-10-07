package depmap

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

// CipherProvider defines the interface for key container encryption and decryption algorithms.
type CipherProvider interface {
	Name() string
	Encrypt(plaintext []byte, password string) (salt, nonce, ciphertext []byte, err error)
	Decrypt(salt, nonce, ciphertext []byte, password string) (plaintext []byte, err error)
}

// AES256GCMProvider implements CipherProvider using AES-256-GCM and PBKDF2-HMAC-SHA256 key derivation.
type AES256GCMProvider struct {
	iterations int
	keySize    int
}

// NewAES256GCMProvider creates a new AES256GCMProvider with default parameters (100,000 iterations, 32-byte key).
func NewAES256GCMProvider() *AES256GCMProvider {
	return &AES256GCMProvider{
		iterations: PBKDF2Iterations,
		keySize:    KeySize,
	}
}

// Name returns the provider identifier.
func (p *AES256GCMProvider) Name() string {
	return "AES-256-GCM"
}

// Encrypt derives an AES-256 key from password and random salt, then encrypts plaintext with AES-GCM.
func (p *AES256GCMProvider) Encrypt(plaintext []byte, password string) ([]byte, []byte, []byte, error) {
	if password == "" {
		return nil, nil, nil, ErrEmptyPassword
	}

	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate random salt: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}

	iterations := p.iterations
	if iterations <= 0 {
		iterations = PBKDF2Iterations
	}
	keySize := p.keySize
	if keySize <= 0 {
		keySize = KeySize
	}

	key := DerivePBKDF2Key([]byte(password), salt, iterations, keySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create GCM block: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return salt, nonce, ciphertext, nil
}

// Decrypt verifies inputs, derives the key, and decrypts ciphertext using AES-GCM.
func (p *AES256GCMProvider) Decrypt(salt, nonce, ciphertext []byte, password string) ([]byte, error) {
	if len(salt) != SaltSize {
		return nil, fmt.Errorf("%w: invalid salt length %d (expected %d)", ErrInvalidFormat, len(salt), SaltSize)
	}
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("%w: invalid nonce length %d (expected %d)", ErrInvalidFormat, len(nonce), NonceSize)
	}
	if len(ciphertext) < TagSize {
		return nil, fmt.Errorf("%w: ciphertext is too short", ErrInvalidFormat)
	}

	iterations := p.iterations
	if iterations <= 0 {
		iterations = PBKDF2Iterations
	}
	keySize := p.keySize
	if keySize <= 0 {
		keySize = KeySize
	}

	key := DerivePBKDF2Key([]byte(password), salt, iterations, keySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
	}

	return plaintext, nil
}

// DefaultCipherProvider is the standard provider used for .depmap containers.
var DefaultCipherProvider CipherProvider = NewAES256GCMProvider()

const passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateRandomPassword creates a cryptographically strong random alphanumeric password of given length.
func GenerateRandomPassword(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("password length must be greater than zero")
	}

	maxVal := big.NewInt(int64(len(passwordCharset)))
	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, maxVal)
		if err != nil {
			return "", fmt.Errorf("failed to generate random character: %w", err)
		}
		buf[i] = passwordCharset[n.Int64()]
	}
	return string(buf), nil
}
