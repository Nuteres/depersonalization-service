package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AuditRecord describes an audit trail entry for depersonalization or restoration operations
// according to Order 140 of Roskomnadzor (152-FZ compliance).
type AuditRecord struct {
	Timestamp    string   `json:"timestamp"`
	Username     string   `json:"username"`
	Hostname     string   `json:"hostname"`
	Operation    string   `json:"operation"` // "depersonalize" | "restore"
	SourceFile   string   `json:"source_file"`
	SourceHash   string   `json:"source_hash"`
	ResultFile   string   `json:"result_file"`
	ResultHash   string   `json:"result_hash"`
	RecordsCount int      `json:"records_count"`
	Entities     []string `json:"entities,omitempty"`
	KeyCreated   bool     `json:"key_created"`
	Status       string   `json:"status"` // "success" | "canceled" | "error"
	Error        string   `json:"error,omitempty"`
}

// AuditLogger manages an append-only, secure audit log in JSON Lines format.
type AuditLogger struct {
	path string
	mu   sync.Mutex
}

// setSecurePerm applies filesystem permissions, ignoring errors on Windows where POSIX chmod is unsupported.
func setSecurePerm(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		if runtime.GOOS == "windows" {
			return nil
		}
		return fmt.Errorf("failed to set permissions %04o on %s: %w", perm, path, err)
	}
	return nil
}

func migrateLegacyAuditLog(legacyPath, defaultPath string) error {
	if err := os.Rename(legacyPath, defaultPath); err == nil {
		return nil
	}
	// Fallback for cross-filesystem moves
	in, err := os.Open(legacyPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(defaultPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	_ = in.Close()
	_ = out.Close()
	_ = setSecurePerm(defaultPath, 0600)
	_ = os.Remove(legacyPath)
	return nil
}

// DefaultAuditPath returns the standard audit log path (~/.depersonalizer/audit.jsonl).
// If legacy path ~/.dep_service/audit.jsonl exists and ~/.depersonalizer/audit.jsonl does not,
// it migrates it to ~/.depersonalizer/audit.jsonl or falls back to the legacy path.
func DefaultAuditPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	defaultDir := filepath.Join(home, ".depersonalizer")
	defaultPath := filepath.Join(defaultDir, "audit.jsonl")
	legacyPath := filepath.Join(home, ".dep_service", "audit.jsonl")

	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, nil
	}

	if _, err := os.Stat(legacyPath); err == nil {
		if err := os.MkdirAll(defaultDir, 0700); err == nil {
			_ = setSecurePerm(defaultDir, 0700)
			if err := migrateLegacyAuditLog(legacyPath, defaultPath); err == nil {
				return defaultPath, nil
			}
		}
		return legacyPath, nil
	}

	return defaultPath, nil
}

// DefaultLogger creates an AuditLogger using the default path (~/.depersonalizer/audit.jsonl).
func DefaultLogger() (*AuditLogger, error) {
	p, err := DefaultAuditPath()
	if err != nil {
		return nil, err
	}
	return NewLogger(p)
}

// NewLogger creates a new AuditLogger writing to path.
// The parent directory is created with secure permissions 0700.
func NewLogger(path string) (*AuditLogger, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("audit log path cannot be empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit directory %s: %w", dir, err)
	}
	if err := setSecurePerm(dir, 0700); err != nil {
		return nil, err
	}

	return &AuditLogger{
		path: path,
	}, nil
}

// Path returns the filesystem path of the audit log.
func (l *AuditLogger) Path() string {
	return l.path
}

// Log writes an AuditRecord to the log file in append-only mode.
// Any empty metadata fields (Timestamp, Username, Hostname) are populated automatically.
// The file is opened with permissions 0600 and flushed with Sync.
func (l *AuditLogger) Log(r AuditRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if r.Timestamp == "" {
		r.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if r.Username == "" {
		r.Username = currentUsername()
	}
	if r.Hostname == "" {
		r.Hostname = currentHostname()
	}

	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create audit directory: %w", err)
	}
	if err := setSecurePerm(dir, 0700); err != nil {
		return err
	}

	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("failed to marshal audit record: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}
	defer f.Close()

	if err := setSecurePerm(l.path, 0600); err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write audit log record: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync audit log file: %w", err)
	}

	return nil
}

// ReadRecent returns the most recent audit records in reverse chronological order (newest first).
// If limit <= 0, all records are returned.
func (l *AuditLogger) ReadRecent(limit int) ([]AuditRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditRecord{}, nil
		}
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	var records []AuditRecord
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r AuditRecord
		if err := json.Unmarshal(line, &r); err != nil {
			// Skip malformed lines rather than failing entire read
			continue
		}
		records = append(records, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed reading audit log: %w", err)
	}

	n := len(records)
	reversed := make([]AuditRecord, 0, n)
	for i := n - 1; i >= 0; i-- {
		reversed = append(reversed, records[i])
		if limit > 0 && len(reversed) >= limit {
			break
		}
	}

	return reversed, nil
}

// ExportTo copies the audit log file to the specified destPath.
func (l *AuditLogger) ExportTo(destPath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if strings.TrimSpace(destPath) == "" {
		return errors.New("destination path cannot be empty")
	}

	src, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(destPath, []byte{}, 0644)
		}
		return fmt.Errorf("failed to open source audit log: %w", err)
	}
	defer src.Close()

	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy audit log: %w", err)
	}

	return dst.Sync()
}

// ComputeFileSHA256 computes the hex-encoded SHA-256 hash of a file
// using buffered streaming without loading the whole file into RAM.
func ComputeFileSHA256(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("file path cannot be empty")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hash calculation: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	reader := bufio.NewReader(f)
	if _, err := io.Copy(h, reader); err != nil {
		return "", fmt.Errorf("failed to compute file hash: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "operator"
}

func currentHostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "localhost"
}
