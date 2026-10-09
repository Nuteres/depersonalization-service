package depmap

import (
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

	// ErrEmptyPassword indicates that a password was not provided for saving.
	ErrEmptyPassword = errors.New("password cannot be empty: use SaveFileWithAutoPassword or provide a password")
)

// DepMap holds the bidirectional mapping metadata and synthetic-to-original dictionary.
type DepMap struct {
	Version          int               `json:"version"`
	CreatedAt        time.Time         `json:"created_at"`
	OriginalFileName string            `json:"original_file_name"`
	RecordsCount     int               `json:"records_count"`
	Mode             string            `json:"mode,omitempty"`
	Mapping          map[string]string `json:"mapping,omitempty"` // synthetic -> original
	Entities         map[string]string `json:"entities,omitempty"`
	CellMappings     map[string]string `json:"cell_mappings,omitempty"`
	TargetHeaders    []string          `json:"target_headers,omitempty"`
	OrdinalMappings  []string          `json:"ordinal_mappings,omitempty"`
}

// New creates a new DepMap initialized with version 1 and current UTC timestamp.
func New(originalFileName string) *DepMap {
	m := make(map[string]string)
	return &DepMap{
		Version:          1,
		CreatedAt:        time.Now().UTC(),
		OriginalFileName: originalFileName,
		RecordsCount:     0,
		Mapping:          m,
		Entities:         m,
	}
}

// Put records a mapping between a synthetic value and the original value.
func (dm *DepMap) Put(synthetic, original string) {
	if dm.Mapping == nil {
		dm.Mapping = make(map[string]string)
	}
	dm.Mapping[synthetic] = original
	dm.Entities = dm.Mapping
	dm.RecordsCount = len(dm.Mapping) + len(dm.CellMappings) + len(dm.OrdinalMappings)
}

// Get looks up the original value for a given synthetic value.
func (dm *DepMap) Get(synthetic string) (string, bool) {
	if dm.Mapping == nil {
		return "", false
	}
	val, ok := dm.Mapping[synthetic]
	return val, ok
}

// LookupOriginal looks up the original value for a given synthetic value.
func (dm *DepMap) LookupOriginal(synthetic string) (string, bool) {
	return dm.Get(synthetic)
}

// PutCell records a mapping for a specific sheet, header, and row to its original value.
func (dm *DepMap) PutCell(sheet, header string, row int, original string) {
	if dm.CellMappings == nil {
		dm.CellMappings = make(map[string]string)
	}
	s := strings.TrimSpace(sheet)
	h := strings.TrimSpace(header)
	var key string
	if s != "" {
		key = fmt.Sprintf("%s!%s:%d", s, h, row)
	} else {
		key = fmt.Sprintf("%s:%d", h, row)
	}
	dm.CellMappings[key] = original
	dm.RecordsCount = len(dm.CellMappings) + len(dm.Mapping) + len(dm.OrdinalMappings)
}

// GetCell retrieves the original value for a specific sheet, header, and row.
func (dm *DepMap) GetCell(sheet, header string, row int) (string, bool) {
	if dm.CellMappings == nil {
		return "", false
	}
	s := strings.TrimSpace(sheet)
	h := strings.TrimSpace(header)
	if s != "" {
		key := fmt.Sprintf("%s!%s:%d", s, h, row)
		if val, ok := dm.CellMappings[key]; ok {
			return val, ok
		}
	}
	keyNoSheet := fmt.Sprintf("%s:%d", h, row)
	if val, ok := dm.CellMappings[keyNoSheet]; ok {
		return val, ok
	}
	keyWithBang := fmt.Sprintf("!%s:%d", h, row)
	val, ok := dm.CellMappings[keyWithBang]
	return val, ok
}

// AddTargetHeader appends a header to TargetHeaders if not already present.
func (dm *DepMap) AddTargetHeader(header string) {
	h := strings.TrimSpace(header)
	if h == "" {
		return
	}
	for _, existing := range dm.TargetHeaders {
		if existing == h {
			return
		}
	}
	dm.TargetHeaders = append(dm.TargetHeaders, h)
}

// PutOrdinal appends an original value to OrdinalMappings.
func (dm *DepMap) PutOrdinal(original string) {
	dm.OrdinalMappings = append(dm.OrdinalMappings, original)
	dm.RecordsCount = len(dm.CellMappings) + len(dm.Mapping) + len(dm.OrdinalMappings)
}

// SaveFileWithAutoPassword serializes the DepMap to zlib-compressed JSON, encrypts it
// using an auto-generated random password if password is empty (or the provided password),
// and returns the effective password used.
func (dm *DepMap) SaveFileWithAutoPassword(path string, password string) (string, error) {
	effectivePassword := password
	if strings.TrimSpace(effectivePassword) == "" {
		generated, err := GenerateRandomPassword(16)
		if err != nil {
			return "", fmt.Errorf("failed to generate random password: %w", err)
		}
		effectivePassword = generated
	}

	if err := dm.SaveFile(path, effectivePassword); err != nil {
		return "", err
	}
	return effectivePassword, nil
}

// SaveFile serializes the DepMap to zlib-compressed JSON, encrypts it with AES-256-GCM
// using a key derived via PBKDF2-HMAC-SHA256, and writes it to the specified path.
// If password is empty, ErrEmptyPassword is returned.
func (dm *DepMap) SaveFile(path string, password string) error {
	return dm.SaveFileWithProvider(path, password, DefaultCipherProvider)
}

// SaveFileWithProvider serializes the DepMap to zlib-compressed JSON, encrypts it with
// the specified CipherProvider, and writes it to the specified path.
func (dm *DepMap) SaveFileWithProvider(path string, password string, provider CipherProvider) error {
	if strings.TrimSpace(password) == "" {
		return ErrEmptyPassword
	}
	if provider == nil {
		provider = DefaultCipherProvider
	}

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

	salt, nonce, ciphertext, err := provider.Encrypt(compressed.Bytes(), password)
	if err != nil {
		return fmt.Errorf("failed to encrypt depmap payload: %w", err)
	}

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
// If password is provided, it attempts decryption with that password only.
// If password is empty, it attempts legacy decryption with DefaultMasterSecret.
func LoadFile(path string, password string) (*DepMap, error) {
	return LoadFileWithProvider(path, password, DefaultCipherProvider)
}

// LoadFileWithProvider reads and decrypts a .depmap container using the specified CipherProvider.
func LoadFileWithProvider(path string, password string, provider CipherProvider) (*DepMap, error) {
	if provider == nil {
		provider = DefaultCipherProvider
	}

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

	var compressed []byte
	if strings.TrimSpace(password) != "" {
		compressed, err = provider.Decrypt(salt, nonce, ciphertext, password)
		if err != nil {
			return nil, err
		}
	} else {
		// Legacy fallback: files created prior to v1.6.0 with empty password used DefaultMasterSecret.
		compressed, err = provider.Decrypt(salt, nonce, ciphertext, DefaultMasterSecret)
		if err != nil {
			return nil, fmt.Errorf("%w: legacy decryption failed", ErrAuthenticationFailed)
		}
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
	dm.Entities = dm.Mapping

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
