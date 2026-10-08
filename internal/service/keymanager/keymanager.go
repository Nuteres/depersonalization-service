package keymanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
)

// KeyManager handles .depmap key container operations: inspection, creation, and mapping extraction.
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

// ExtractMappings extracts bidirectional mappings between original input file and anonymized output file.
// It supports CSV, TSV, and Excel spreadsheet formats.
func (km *KeyManager) ExtractMappings(inputPath, outputPath, ext string, dm *depmap.DepMap) error {
	if dm == nil {
		return errors.New("depmap cannot be nil")
	}

	if ext == "" {
		ext = filepath.Ext(inputPath)
	}
	ext = strings.ToLower(ext)

	switch ext {
	case ".csv", ".tsv":
		return extractCSVMappings(inputPath, outputPath, dm)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return extractExcelMappings(inputPath, outputPath, dm)
	default:
		return nil
	}
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


func extractCSVMappings(inputPath, outputPath string, dm *depmap.DepMap) error {
	inFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input csv for mapping: %w", err)
	}
	defer inFile.Close()

	outFile, err := os.Open(outputPath)
	if err != nil {
		return fmt.Errorf("failed to open output csv for mapping: %w", err)
	}
	defer outFile.Close()

	inReader, _, err := formats.NewEncodingAwareCSVReader(inFile, 0)
	if err != nil {
		return fmt.Errorf("failed to open input csv reader: %w", err)
	}

	outReader, _, err := formats.NewEncodingAwareCSVReader(outFile, 0)
	if err != nil {
		return fmt.Errorf("failed to open output csv reader: %w", err)
	}

	for {
		inRec, inErr := inReader.Read()
		outRec, outErr := outReader.Read()

		if inErr != nil || outErr != nil {
			break
		}

		limit := len(inRec)
		if len(outRec) < limit {
			limit = len(outRec)
		}

		for i := 0; i < limit; i++ {
			origVal := strings.TrimPrefix(inRec[i], "\ufeff")
			anonVal := strings.TrimPrefix(outRec[i], "\ufeff")
			if anonVal != origVal && strings.TrimSpace(origVal) != "" {
				dm.Put(anonVal, origVal)
				if strings.TrimSpace(anonVal) != anonVal {
					dm.Put(strings.TrimSpace(anonVal), origVal)
				}
			}
		}
	}

	return nil
}

func extractExcelMappings(inputPath, outputPath string, dm *depmap.DepMap) error {
	inXl, err := excelize.OpenFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input excel for mapping: %w", err)
	}
	defer inXl.Close()

	outXl, err := excelize.OpenFile(outputPath)
	if err != nil {
		return fmt.Errorf("failed to open output excel for mapping: %w", err)
	}
	defer outXl.Close()

	for _, sheet := range inXl.GetSheetList() {
		inRows, err := inXl.Rows(sheet)
		if err != nil {
			continue
		}

		outRows, err := outXl.Rows(sheet)
		if err != nil {
			_ = inRows.Close()
			continue
		}

		for inRows.Next() && outRows.Next() {
			inCols, errIn := inRows.Columns()
			outCols, errOut := outRows.Columns()
			if errIn != nil || errOut != nil {
				break
			}

			limit := len(inCols)
			if len(outCols) < limit {
				limit = len(outCols)
			}

			for i := 0; i < limit; i++ {
				origVal := inCols[i]
				anonVal := outCols[i]
				if anonVal != origVal && strings.TrimSpace(origVal) != "" {
					dm.Put(anonVal, origVal)
					if strings.TrimSpace(anonVal) != anonVal {
						dm.Put(strings.TrimSpace(anonVal), origVal)
					}
				}
			}
		}

		_ = inRows.Close()
		_ = outRows.Close()
	}

	return nil
}
