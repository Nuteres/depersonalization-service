package depmap

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	// MagicHeader is the 8-byte file identifier for .depmap containers.
	MagicHeader = "DEPMAP01"

	// SaltSize is the size in bytes of the random salt for PBKDF2.
	SaltSize = 16

	// NonceSize is the standard 12-byte nonce for AES-GCM.
	NonceSize = 12

	// TagSize is the 16-byte authentication tag produced by AES-GCM.
	TagSize = 16

	// HeaderSize is the fixed size before ciphertext (magic + salt + nonce).
	HeaderSize = len(MagicHeader) + SaltSize + NonceSize

	// MinFileSize is the minimum valid file size (header + GCM tag).
	MinFileSize = HeaderSize + TagSize

	// PBKDF2Iterations is the iteration count for key derivation.
	PBKDF2Iterations = 100000

	// KeySize is the 256-bit AES key size (32 bytes).
	KeySize = 32

	// DefaultMasterSecret is used when no user password is provided.
	DefaultMasterSecret = "dep_service_152fz_master_depmap_secret_2026"
)

var (
	// ErrInvalidFormat indicates that the file is not a valid .depmap container.
	ErrInvalidFormat = errors.New("invalid .depmap file format")

	// ErrAuthenticationFailed indicates that decryption or GCM verification failed.
	ErrAuthenticationFailed = errors.New("authentication failed: incorrect password or corrupted file")

	// ErrFileTooShort indicates that the file is too small to contain valid metadata.
	ErrFileTooShort = errors.New("file is too short to be a valid .depmap file")
)

// DepMap holds the bidirectional mapping metadata and synthetic-to-original dictionary.
type DepMap struct {
	Version          int               `json:"version"`
	CreatedAt        time.Time         `json:"created_at"`
	OriginalFileName string            `json:"original_file_name"`
	RecordsCount     int               `json:"records_count"`
	Mapping          map[string]string `json:"mapping"` // synthetic -> original
}

// New creates a new DepMap initialized with version 1 and current UTC timestamp.
func New(originalFileName string) *DepMap {
	return &DepMap{
		Version:          1,
		CreatedAt:        time.Now().UTC(),
		OriginalFileName: originalFileName,
		RecordsCount:     0,
		Mapping:          make(map[string]string),
	}
}

// Put records a mapping between a synthetic value and the original value.
func (dm *DepMap) Put(synthetic, original string) {
	if dm.Mapping == nil {
		dm.Mapping = make(map[string]string)
	}
	dm.Mapping[synthetic] = original
	dm.RecordsCount = len(dm.Mapping)
}

// Get looks up the original value for a given synthetic value.
func (dm *DepMap) Get(synthetic string) (string, bool) {
	if dm.Mapping == nil {
		return "", false
	}
	val, ok := dm.Mapping[synthetic]
	return val, ok
}

// SaveFile serializes the DepMap to zlib-compressed JSON, encrypts it with AES-256-GCM
// using a key derived via PBKDF2-HMAC-SHA256, and writes it to the specified path.
func (dm *DepMap) SaveFile(path string, password string) error {
	dm.RecordsCount = len(dm.Mapping)

	jsonData, err := json.Marshal(dm)
	if err != nil {
		return fmt.Errorf("failed to marshal depmap: %w", err)
	}

	// Compress JSON with zlib
	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.DefaultCompression)
	if err != nil {
		return fmt.Errorf("failed to initialize zlib writer: %w", err)
	}
	if _, err := zw.Write(jsonData); err != nil {
		_ = zw.Close()
		return fmt.Errorf("failed to compress depmap payload: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalize zlib compression: %w", err)
	}

	// Generate random salt and nonce
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate random salt: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate random nonce: %w", err)
	}

	// Derive AES-256 key
	keySecret := password
	if keySecret == "" {
		keySecret = DefaultMasterSecret
	}
	key := DerivePBKDF2Key([]byte(keySecret), salt, PBKDF2Iterations, KeySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM block: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, compressed.Bytes(), nil)

	// Ensure destination directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write container to file: Magic(8) + Salt(16) + Nonce(12) + Ciphertext+Tag
	var buf bytes.Buffer
	buf.Grow(HeaderSize + len(ciphertext))
	buf.WriteString(MagicHeader)
	buf.Write(salt)
	buf.Write(nonce)
	buf.Write(ciphertext)

	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write depmap file: %w", err)
	}

	return nil
}

// LoadFile reads and decrypts a .depmap container from the specified path.
func LoadFile(path string, password string) (*DepMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read depmap file: %w", err)
	}

	if len(data) < MinFileSize {
		return nil, ErrFileTooShort
	}

	if string(data[:len(MagicHeader)]) != MagicHeader {
		return nil, fmt.Errorf("%w: invalid magic signature", ErrInvalidFormat)
	}

	offset := len(MagicHeader)
	salt := data[offset : offset+SaltSize]
	offset += SaltSize

	nonce := data[offset : offset+NonceSize]
	offset += NonceSize

	ciphertext := data[offset:]

	// Derive key
	keySecret := password
	if keySecret == "" {
		keySecret = DefaultMasterSecret
	}
	key := DerivePBKDF2Key([]byte(keySecret), salt, PBKDF2Iterations, KeySize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}

	compressed, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
	}

	// Decompress payload
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize zlib decompression: %w", err)
	}
	defer zr.Close()

	jsonData, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress depmap payload: %w", err)
	}

	var dm DepMap
	if err := json.Unmarshal(jsonData, &dm); err != nil {
		return nil, fmt.Errorf("failed to parse depmap JSON payload: %w", err)
	}

	if dm.Mapping == nil {
		dm.Mapping = make(map[string]string)
	}

	return &dm, nil
}

// InspectKeyFile validates the header and structure of a .depmap file without requiring full decryption.
func InspectKeyFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return false, err
	}

	if fi.Size() < int64(MinFileSize) {
		return false, ErrFileTooShort
	}

	header := make([]byte, len(MagicHeader))
	if _, err := io.ReadFull(f, header); err != nil {
		return false, fmt.Errorf("failed to read magic header: %w", err)
	}

	if string(header) != MagicHeader {
		return false, fmt.Errorf("%w: unexpected magic header", ErrInvalidFormat)
	}

	return true, nil
}

// DerivePBKDF2Key implements RFC 2898 / RFC 8018 PBKDF2 with HMAC-SHA256.
func DerivePBKDF2Key(password, salt []byte, iter, keyLen int) []byte {
	hashLen := sha256.Size // 32
	numBlocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, numBlocks*hashLen)

	h := hmac.New(sha256.New, password)
	var blockBytes [4]byte
	u := make([]byte, hashLen)
	t := make([]byte, hashLen)

	for block := 1; block <= numBlocks; block++ {
		binary.BigEndian.PutUint32(blockBytes[:], uint32(block))
		h.Reset()
		h.Write(salt)
		h.Write(blockBytes[:])
		u = h.Sum(u[:0])
		copy(t, u)

		for j := 1; j < iter; j++ {
			h.Reset()
			h.Write(u)
			u = h.Sum(u[:0])
			for k := 0; k < hashLen; k++ {
				t[k] ^= u[k]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}
