package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/formats"
)

const (
	defaultSalt  = "152fz-secure-salt"
	defaultLimit = 20
	maxLimit     = 100
)

// PreviewService provides row-level preview and dynamic pagination for tabular files.
type PreviewService struct{}

// New creates a new PreviewService instance.
func New() *PreviewService {
	return &PreviewService{}
}

// GetTableRows fetches a window of preview rows starting at offset with limit for the default sheet.
func (s *PreviewService) GetTableRows(ctx context.Context, path string, offset, limit int, salt string, disabledCols []int, mode ...string) ([]domain.PreviewRow, error) {
	m := ""
	if len(mode) > 0 {
		m = mode[0]
	}
	return s.GetSheetRows(ctx, path, "", offset, limit, salt, disabledCols, m)
}

// GetSheetRows fetches a paginated window of rows for a specific sheet in a workbook or tabular file.
// It applies anonymization rules while respecting disabledCols (1-based column indices).
func (s *PreviewService) GetSheetRows(ctx context.Context, path, sheetName string, offset, limit int, salt string, disabledCols []int, mode ...string) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if strings.TrimSpace(path) == "" {
		return nil, errors.New("file path cannot be empty")
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%w: path is a directory", domain.ErrUnsupportedFormat)
	}

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	if strings.TrimSpace(salt) == "" {
		salt = defaultSalt
	}

	strategy := domain.StrategySynthetic
	if len(mode) > 0 && strings.TrimSpace(mode[0]) != "" {
		m := strings.TrimSpace(mode[0])
		switch m {
		case string(domain.StrategyDelete):
			strategy = domain.StrategyDelete
		case string(domain.StrategySemanticTokens):
			strategy = domain.StrategySemanticTokens
		case string(domain.StrategySynthetic):
			strategy = domain.StrategySynthetic
		default:
			strategy = domain.StrategySynthetic
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv", ".tsv":
		return getCSVRows(ctx, path, offset, limit, salt, disabledCols, strategy)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return getXLSXRowsForSheet(ctx, path, sheetName, offset, limit, salt, disabledCols, strategy)
	case ".xls":
		return getXLSRowsForSheet(ctx, path, sheetName, offset, limit, salt, disabledCols, strategy)
	case ".xlsb":
		return getXLSBRowsForSheet(ctx, path, sheetName, offset, limit, salt, disabledCols, strategy)
	case ".ods", ".xods":
		return getODSRowsForSheet(ctx, path, sheetName, offset, limit, salt, disabledCols, strategy)
	default:
		return nil, fmt.Errorf("%w: динамическая подгрузка строк поддерживается только для таблиц: %s", domain.ErrUnsupportedFormat, ext)
	}
}

func getXLSXRows(ctx context.Context, path string, offset, limit int, salt string, disabledCols []int, mode ...domain.ProcessingMode) ([]domain.PreviewRow, error) {
	strat := domain.StrategySemanticTokens
	if len(mode) > 0 && mode[0] == domain.ModeDelete {
		strat = domain.StrategyDelete
	}
	return getXLSXRowsForSheet(ctx, path, "", offset, limit, salt, disabledCols, strat)
}

func getXLSXRowsForSheet(ctx context.Context, path string, targetSheet string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	xl, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsx file: %w", err)
	}
	defer xl.Close()

	sheets := xl.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("xlsx file has no sheets")
	}

	sheetName := sheets[0]
	if targetSheet != "" {
		for _, s := range sheets {
			if s == targetSheet {
				sheetName = s
				break
			}
		}
	}

	// Sample top rows to find header
	sampleIter, err := xl.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}
	var sample [][]string
	for sampleIter.Next() && len(sample) < 40 {
		if ctx != nil && ctx.Err() != nil {
			_ = sampleIter.Close()
			return nil, ctx.Err()
		}
		cols, err := sampleIter.Columns()
		if err != nil {
			break
		}
		sample = append(sample, cols)
	}
	_ = sampleIter.Close()

	if len(sample) == 0 {
		return nil, nil
	}

	headerIdx, headers := formats.FindHeaderRow(sample)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}
	colTypes := formats.ResolveColumnTypesWithSamples(headers, sample[headerIdx+1:], nil)

	disabledMap := make(map[int]bool, len(disabledCols))
	for _, col := range disabledCols {
		disabledMap[col] = true
	}

	anon := anonymizer.New(salt)
	ts := anonymizer.NewTokenSession(salt)

	rowsIter, err := xl.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}
	defer rowsIter.Close()

	targetSkip := headerIdx + 1 + offset
	currentIdx := 0

	var result []domain.PreviewRow
	for rowsIter.Next() {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		currentIdx++
		if currentIdx <= targetSkip {
			if strategy == domain.StrategySemanticTokens && currentIdx > headerIdx+1 {
				cols, err := rowsIter.Columns()
				if err == nil {
					for j := 0; j < len(headers) && j < len(cols); j++ {
						col1Based := j + 1
						if j < len(colTypes) && !disabledMap[col1Based] {
							ct := colTypes[j]
							if ct != formats.ColUnknown {
								_ = formats.ProcessCellWithStrategy(anon, ts, ct, cols[j], strategy, 30)
							}
						}
					}
				}
			}
			continue
		}

		cols, err := rowsIter.Columns()
		if err != nil {
			break
		}

		before := make([]string, len(headers))
		after := make([]string, len(headers))
		for j := 0; j < len(headers); j++ {
			val := ""
			if j < len(cols) {
				val = cols[j]
			}
			before[j] = val
			var ct formats.ColumnType
			col1Based := j + 1
			if j < len(colTypes) && !disabledMap[col1Based] {
				ct = colTypes[j]
			}
			after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, strategy, 30)
		}

		result = append(result, domain.PreviewRow{
			Index:  offset + len(result) + 1,
			Before: before,
			After:  after,
		})

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}


func getCSVRows(ctx context.Context, path string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv file: %w", err)
	}
	defer file.Close()

	csvReader, comma, err := formats.NewEncodingAwareCSVReader(file, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv reader: %w", err)
	}

	var sample [][]string
	for len(sample) < 40 {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rec, err := csvReader.Read()
		if err != nil {
			break
		}
		sample = append(sample, rec)
	}

	if len(sample) == 0 {
		return nil, nil
	}

	headerIdx, headers := formats.FindHeaderRow(sample)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}
	colTypes := formats.ResolveColumnTypesWithSamples(headers, sample[headerIdx+1:], nil)

	disabledMap := make(map[int]bool, len(disabledCols))
	for _, col := range disabledCols {
		disabledMap[col] = true
	}

	anon := anonymizer.New(salt)
	ts := anonymizer.NewTokenSession(salt)

	// Seek back to start cleanly
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek csv file: %w", err)
	}
	csvReader, _, err = formats.NewEncodingAwareCSVReader(file, comma)
	if err != nil {
		return nil, fmt.Errorf("failed to recreate csv reader: %w", err)
	}

	targetSkip := headerIdx + 1 + offset
	currentIdx := 0

	var result []domain.PreviewRow
	for {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rec, err := csvReader.Read()
		if err != nil {
			break
		}
		currentIdx++
		if currentIdx <= targetSkip {
			if strategy == domain.StrategySemanticTokens && currentIdx > headerIdx+1 {
				for j := 0; j < len(headers) && j < len(rec); j++ {
					col1Based := j + 1
					if j < len(colTypes) && !disabledMap[col1Based] {
						ct := colTypes[j]
						if ct != formats.ColUnknown {
							_ = formats.ProcessCellWithStrategy(anon, ts, ct, rec[j], strategy, 30)
						}
					}
				}
			}
			continue
		}

		before := make([]string, len(headers))
		after := make([]string, len(headers))
		for j := 0; j < len(headers); j++ {
			val := ""
			if j < len(rec) {
				val = rec[j]
			}
			before[j] = val
			var ct formats.ColumnType
			col1Based := j + 1
			if j < len(colTypes) && !disabledMap[col1Based] {
				ct = colTypes[j]
			}
			after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, strategy, 30)
		}

		result = append(result, domain.PreviewRow{
			Index:  offset + len(result) + 1,
			Before: before,
			After:  after,
		})

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

func getXLSRowsForSheet(ctx context.Context, path, targetSheet string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	handler := formats.NewXLSHandler(nil)
	fetchLimit := offset + limit + 100
	sheets, samples, err := handler.InspectRows(path, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect xls: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("xls file has no sheets")
	}

	sheetName := sheets[0].Name
	if targetSheet != "" {
		for _, s := range sheets {
			if s.Name == targetSheet {
				sheetName = s.Name
				break
			}
		}
	}

	return paginateSampleRows(samples[sheetName], offset, limit, salt, disabledCols, strategy)
}

func getXLSBRowsForSheet(ctx context.Context, path, targetSheet string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	handler := formats.NewXLSBHandler(nil)
	fetchLimit := offset + limit + 100
	sheets, samples, err := handler.InspectRows(path, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect xlsb: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("xlsb file has no sheets")
	}

	sheetName := sheets[0].Name
	if targetSheet != "" {
		for _, s := range sheets {
			if s.Name == targetSheet {
				sheetName = s.Name
				break
			}
		}
	}

	return paginateSampleRows(samples[sheetName], offset, limit, salt, disabledCols, strategy)
}

func getODSRowsForSheet(ctx context.Context, path, targetSheet string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ods file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat ods file: %w", err)
	}

	handler := formats.NewODSHandler(nil)
	fetchLimit := offset + limit + 100
	sheets, samples, err := handler.InspectRows(f, stat.Size(), fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect ods: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("ods file has no sheets")
	}

	sheetName := sheets[0].Name
	if targetSheet != "" {
		for _, s := range sheets {
			if s.Name == targetSheet {
				sheetName = s.Name
				break
			}
		}
	}

	return paginateSampleRows(samples[sheetName], offset, limit, salt, disabledCols, strategy)
}

func paginateSampleRows(sampleRows [][]string, offset, limit int, salt string, disabledCols []int, strategy domain.AnonymizationStrategy) ([]domain.PreviewRow, error) {
	if len(sampleRows) == 0 {
		return nil, nil
	}

	headerIdx, headers := formats.FindHeaderRow(sampleRows)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}
	colTypes := formats.ResolveColumnTypesWithSamples(headers, sampleRows[headerIdx+1:], nil)

	disabledMap := make(map[int]bool, len(disabledCols))
	for _, col := range disabledCols {
		disabledMap[col] = true
	}

	anon := anonymizer.New(salt)
	ts := anonymizer.NewTokenSession(salt)
	startRow := headerIdx + 1 + offset

	if strategy == domain.StrategySemanticTokens {
		for r := headerIdx + 1; r < startRow && r < len(sampleRows); r++ {
			cols := sampleRows[r]
			for j := 0; j < len(headers) && j < len(cols); j++ {
				col1Based := j + 1
				if j < len(colTypes) && !disabledMap[col1Based] {
					ct := colTypes[j]
					if ct != formats.ColUnknown {
						_ = formats.ProcessCellWithStrategy(anon, ts, ct, cols[j], strategy, 30)
					}
				}
			}
		}
	}

	var result []domain.PreviewRow
	for r := startRow; r < len(sampleRows) && len(result) < limit; r++ {
		cols := sampleRows[r]
		before := make([]string, len(headers))
		after := make([]string, len(headers))
		for j := 0; j < len(headers); j++ {
			val := ""
			if j < len(cols) {
				val = cols[j]
			}
			before[j] = val
			var ct formats.ColumnType
			col1Based := j + 1
			if j < len(colTypes) && !disabledMap[col1Based] {
				ct = colTypes[j]
			}
			after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, strategy, 30)
		}

		result = append(result, domain.PreviewRow{
			Index:  offset + len(result) + 1,
			Before: before,
			After:  after,
		})
	}

	return result, nil
}
