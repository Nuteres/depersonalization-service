package keymanager

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"dep_service/internal/domain"
	"dep_service/pkg/depmap"
)

// KeyManager handles .depmap key container operations: inspection and container creation.
type KeyManager struct{}

// New creates a new KeyManager instance.
func New() *KeyManager {
	return &KeyManager{}
}

// InspectKey inspects a .depmap key file, validates its header, and tests password if provided.
// It wraps invalid password errors with domain.ErrInvalidPassword.
func (km *KeyManager) InspectKey(keyPath, password string) (*domain.KeyFileInfo, error) {
	if strings.TrimSpace(keyPath) == "" {
		return nil, errors.New("key path cannot be empty")
	}

	if _, err := os.Stat(keyPath); err != nil {
		return nil, fmt.Errorf("key file not found: %w", err)
	}

	valid, err := depmap.InspectKeyFile(keyPath)
	if err != nil || !valid {
		errMsg := "неверный формат файла ключа .depmap"
		if err != nil {
			errMsg = err.Error()
		}
		return &domain.KeyFileInfo{
			Path:  keyPath,
			Valid: false,
			Error: errMsg,
		}, nil
	}

	dm, err := depmap.LoadFile(keyPath, password)
	if err != nil {
		wrappedErr := err
		if errors.Is(err, depmap.ErrAuthenticationFailed) {
			wrappedErr = fmt.Errorf("%w: %v", domain.ErrInvalidPassword, err)
		}
		return &domain.KeyFileInfo{
			Path:  keyPath,
			Valid: false,
			Error: wrappedErr.Error(),
		}, wrappedErr
	}

	return &domain.KeyFileInfo{
		Path:             keyPath,
		OriginalFileName: dm.OriginalFileName,
		CreatedAt:        dm.CreatedAt.Format("2006-01-02 15:04:05"),
		RecordsCount:     dm.RecordsCount,
		Valid:            true,
	}, nil
}

// CreateContainer creates and writes an encrypted .depmap key container file.
func (km *KeyManager) CreateContainer(keyPath, password, origFileName string, mappings map[string]string) error {
	if strings.TrimSpace(keyPath) == "" {
		return errors.New("key path cannot be empty")
	}

	dm := depmap.New(origFileName)
	for k, v := range mappings {
		dm.Put(k, v)
	}

	return dm.SaveFile(keyPath, password)
}

