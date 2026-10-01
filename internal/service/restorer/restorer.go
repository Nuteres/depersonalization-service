package restorer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dep_service/internal/domain"
	"dep_service/internal/service/keymanager"
	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
)

// Restorer handles reverse depersonalization of previously anonymized files using .depmap keys.
type Restorer struct {
	km *keymanager.KeyManager
}

// RestorerService is an alias to Restorer for architectural consistency.
type RestorerService = Restorer

// New creates a new Restorer service.
func New(km ...*keymanager.KeyManager) *Restorer {
	var k *keymanager.KeyManager
	if len(km) > 0 && km[0] != nil {
		k = km[0]
	} else {
		k = keymanager.New()
	}
	return &Restorer{km: k}
}

// Restore restores original sensitive data in inputPath using keyPath (.depmap),
// writing the restored file to destPath, and calling reporter with progress updates.
// It wraps invalid password with domain.ErrInvalidPassword and unsupported formats with domain.ErrUnsupportedFormat.
func (r *Restorer) Restore(ctx context.Context, opts domain.RestoreOptions, reporter domain.ProgressReporter) (*domain.RestoreResult, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	inputPath := opts.InputPath
	keyPath := opts.KeyPath
	password := opts.Password
	destPath := opts.OutputPath

	if strings.TrimSpace(inputPath) == "" {
		return nil, errors.New("input file path cannot be empty")
	}
	if strings.TrimSpace(keyPath) == "" {
		return nil, errors.New("key file path cannot be empty")
	}

	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("input file not found: %w", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return nil, fmt.Errorf("key file not found: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(inputPath))
	if strings.TrimSpace(destPath) == "" {
		dir := filepath.Dir(inputPath)
		base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
		if strings.HasSuffix(base, "_anonymized") {
			base = strings.TrimSuffix(base, "_anonymized")
		}
		destPath = filepath.Join(dir, base+"_restored"+ext)
	}

	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	dm, err := depmap.LoadFile(keyPath, password)
	if err != nil {
		if errors.Is(err, depmap.ErrAuthenticationFailed) {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidPassword, err)
		}
		return nil, fmt.Errorf("failed to load key file: %w", err)
	}

	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if ctx == nil {
		ctx = context.Background()
	}

	startTime := time.Now()
	var restoredCount int

	switch ext {
	case ".csv", ".tsv":
		inFile, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		defer inFile.Close()

		outFile, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}
		defer outFile.Close()

		restoredCount, err = formats.RestoreCSV(ctx, inFile, outFile, dm)
		if err != nil {
			_ = os.Remove(destPath)
			if errors.Is(err, context.Canceled) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to restore csv: %w", err)
		}

	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		var onProg func(pct int, proc, total int)
		if reporter != nil {
			onProg = func(pct int, proc, total int) {
				reporter(proc, total, pct)
			}
		}
		restoredCount, err = formats.RestoreExcel(ctx, inputPath, destPath, dm, onProg)
		if err != nil {
			_ = os.Remove(destPath)
			if errors.Is(err, context.Canceled) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to restore excel: %w", err)
		}

	case ".ods":
		inStat, err := os.Stat(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat input file: %w", err)
		}

		inFile, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		defer inFile.Close()

		outFile, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}
		defer outFile.Close()

		restoredCount, err = formats.RestoreODS(ctx, inFile, inStat.Size(), outFile, dm)
		if err != nil {
			_ = os.Remove(destPath)
			if errors.Is(err, context.Canceled) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to restore ods: %w", err)
		}

	case ".docx":
		inStat, err := os.Stat(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat input file: %w", err)
		}

		inFile, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		defer inFile.Close()

		outFile, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}
		defer outFile.Close()

		restoredCount, err = formats.RestoreDOCX(ctx, inFile, inStat.Size(), outFile, dm)
		if err != nil {
			_ = os.Remove(destPath)
			if errors.Is(err, context.Canceled) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to restore docx: %w", err)
		}

	case ".odt":
		inStat, err := os.Stat(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat input file: %w", err)
		}

		inFile, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		defer inFile.Close()

		outFile, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}
		defer outFile.Close()

		restoredCount, err = formats.RestoreODT(ctx, inFile, inStat.Size(), outFile, dm)
		if err != nil {
			_ = os.Remove(destPath)
			if errors.Is(err, context.Canceled) || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to restore odt: %w", err)
		}

	default:
		return nil, fmt.Errorf("%w: unsupported file extension for restoration: %s", domain.ErrUnsupportedFormat, ext)
	}

	if ctx != nil && ctx.Err() != nil {
		_ = os.Remove(destPath)
		return nil, ctx.Err()
	}

	if reporter != nil {
		reporter(restoredCount, restoredCount, 100)
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &domain.RestoreResult{
		Success:       true,
		InputPath:     inputPath,
		KeyPath:       keyPath,
		OutputPath:    destPath,
		RestoredCells: restoredCount,
		DurationMs:    durationMs,
	}, nil
}
