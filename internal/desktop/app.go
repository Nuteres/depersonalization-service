package desktop

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	goruntime "runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
	"dep_service/internal/version"
	"dep_service/pkg/audit"
	"dep_service/pkg/depmap"
	"dep_service/pkg/ziputil"
)

// DesktopApp is the presentation adapter struct bound to the Wails runtime.
type DesktopApp struct {
	ctx      context.Context
	insp     *inspector.Inspector
	prev     *preview.PreviewService
	proc     *processor.Processor
	rest     *restorer.Restorer
	km       *keymanager.KeyManager
	audit    *audit.AuditLogger
	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

// New creates a new DesktopApp instance wired with the provided services.
func New(
	insp *inspector.Inspector,
	prev *preview.PreviewService,
	proc *processor.Processor,
	rest *restorer.Restorer,
	km *keymanager.KeyManager,
	al ...*audit.AuditLogger,
) *DesktopApp {
	if km == nil {
		km = keymanager.New()
	}
	if insp == nil {
		insp = inspector.New()
	}
	if prev == nil {
		prev = preview.New()
	}
	var auditLog *audit.AuditLogger
	if len(al) > 0 && al[0] != nil {
		auditLog = al[0]
	} else {
		auditLog, _ = audit.DefaultLogger()
	}

	if proc == nil {
		proc = processor.New(km, insp)
	}
	if auditLog != nil {
		proc.SetAuditLogger(auditLog)
	}

	if rest == nil {
		rest = restorer.New(km)
	}
	if auditLog != nil {
		rest.SetAuditLogger(auditLog)
	}

	return &DesktopApp{
		insp:  insp,
		prev:  prev,
		proc:  proc,
		rest:  rest,
		km:    km,
		audit: auditLog,
	}
}

// Startup is called at application startup by Wails.
func (a *DesktopApp) Startup(ctx context.Context) {
	a.ctx = ctx
	if ctx != nil && ctx.Value("events") != nil {
		runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
			if len(paths) > 0 && paths[0] != "" {
				emitFileDropped(ctx, paths[0])
			}
		})
	}
}

// GetAppInfo returns system environment metadata (current operating system, architecture, app version).
func (a *DesktopApp) GetAppInfo() map[string]string {
	return map[string]string{
		"version": "v" + version.Version,
		"os":      detectOS(),
		"arch":    goruntime.GOARCH,
	}
}

// InspectFile inspects the given file and returns its domain.FileInfo.
func (a *DesktopApp) InspectFile(path string) (*domain.FileInfo, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.insp.Inspect(ctx, path)
}

// GetFileBase64 reads the file at path and returns its contents encoded as a base64 string.
func (a *DesktopApp) GetFileBase64(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if stat.Size() > domain.MaxBase64PreviewFileSize {
		return "", fmt.Errorf("%w: размер файла превышает лимит безопасности для предпросмотра (100 МБ)", domain.ErrFileTooLarge)
	}

	limited := io.LimitReader(f, domain.MaxBase64PreviewFileSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if int64(len(data)) > domain.MaxBase64PreviewFileSize {
		return "", fmt.Errorf("%w: размер файла превышает лимит безопасности для предпросмотра (100 МБ)", domain.ErrFileTooLarge)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// GetTableRows fetches a window of preview rows starting at offset with limit for the default sheet.
func (a *DesktopApp) GetTableRows(path string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error) {
	return a.GetSheetRows(path, "", offset, limit, salt, disabledCols, "")
}

// GetSheetRows fetches a paginated window of rows for a specific sheet.
func (a *DesktopApp) GetSheetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int, mode string) ([]domain.PreviewRow, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.prev.GetSheetRows(ctx, path, sheetName, offset, limit, salt, disabledCols, mode)
}

// ProcessFile processes the input file with default output path and options.
func (a *DesktopApp) ProcessFile(path, salt string, mode ...string) (*domain.ProcessResult, error) {
	m := ""
	if len(mode) > 0 {
		m = mode[0]
	}
	return a.ProcessFileTo(path, "", salt, nil, false, "", "", false, m)
}

// ProcessFileTo processes the input file and writes to outPath.
func (a *DesktopApp) ProcessFileTo(
	path, outPath, salt string,
	disabledCols []int,
	createKey bool,
	keyPassword, keyPath string,
	shuffleRows bool,
	mode ...string,
) (*domain.ProcessResult, error) {
	m := ""
	if len(mode) > 0 {
		m = mode[0]
	}
	return a.ProcessFileWithSheets(path, outPath, salt, disabledCols, nil, createKey, keyPassword, keyPath, shuffleRows, m)
}

// ProcessFileWithSheets processes the input file with per-sheet disabled column options and optional mode.
func (a *DesktopApp) ProcessFileWithSheets(
	path string,
	outPath string,
	salt string,
	disabledCols []int,
	disabledBySheet map[string][]int,
	createKey bool,
	keyPassword string,
	keyPath string,
	shuffleRows bool,
	mode string,
) (*domain.ProcessResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithCancel(ctx)
	a.cancelMu.Lock()
	a.cancel = cancel
	a.cancelMu.Unlock()
	defer func() {
		a.cancelMu.Lock()
		a.cancel = nil
		a.cancelMu.Unlock()
		cancel()
	}()

	processingMode := domain.ModeReplace
	strategy := domain.StrategySemanticTokens

	m := strings.TrimSpace(mode)
	if m != "" {
		switch m {
		case string(domain.StrategyDelete):
			processingMode = domain.ModeDelete
			strategy = domain.StrategyDelete
		case string(domain.StrategySynthetic):
			processingMode = domain.ModeReplace
			strategy = domain.StrategySynthetic
		case string(domain.StrategySemanticTokens):
			processingMode = domain.ModeReplace
			strategy = domain.StrategySemanticTokens
		default:
			processingMode = domain.ProcessingMode(m)
			strategy = domain.StrategySemanticTokens
		}
	}

	reporter := newProcessProgressReporter(a.ctx)
	opts := domain.ProcessOptions{
		InputPath:       path,
		OutputPath:      outPath,
		Mode:            processingMode,
		Strategy:        strategy,
		Salt:            salt,
		DisabledCols:    disabledCols,
		DisabledBySheet: disabledBySheet,
		CreateKey:       createKey,
		KeyPassword:     keyPassword,
		KeyPath:         keyPath,
		ShuffleRows:     shuffleRows,
	}

	return a.proc.Process(opCtx, opts, reporter)
}

// ProcessFileWithOptions processes the input file with full domain.ProcessOptions.
func (a *DesktopApp) ProcessFileWithOptions(opts domain.ProcessOptions) (*domain.ProcessResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithCancel(ctx)
	a.cancelMu.Lock()
	a.cancel = cancel
	a.cancelMu.Unlock()
	defer func() {
		a.cancelMu.Lock()
		a.cancel = nil
		a.cancelMu.Unlock()
		cancel()
		goruntime.GC()
		debug.FreeOSMemory()
	}()

	if opts.Strategy == "" {
		if opts.Mode == domain.ModeDelete {
			opts.Strategy = domain.StrategyDelete
		} else {
			opts.Strategy = domain.StrategySemanticTokens
		}
	}

	reporter := newProcessProgressReporter(a.ctx)
	return a.proc.Process(opCtx, opts, reporter)
}

// CancelCurrentOperation cancels the currently active processing or restoration operation.
func (a *DesktopApp) CancelCurrentOperation() {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

// InspectKeyFile inspects a .depmap key file, validates its header, and tests password if provided.
func (a *DesktopApp) InspectKeyFile(keyPath, password string) (*domain.KeyFileInfo, error) {
	defer func() {
		goruntime.GC()
		debug.FreeOSMemory()
	}()
	info, err := a.km.InspectKey(keyPath, password)
	if err != nil {
		if info != nil && !info.Valid {
			return info, nil
		}
		return nil, err
	}
	return info, nil
}

// RestoreFileTo restores original personal data in inputPath using keyPath (.depmap),
// writing the restored file to destPath, and emitting restore-progress events.
func (a *DesktopApp) RestoreFileTo(inputPath, keyPath, password, destPath string) (*domain.RestoreResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithCancel(ctx)
	a.cancelMu.Lock()
	a.cancel = cancel
	a.cancelMu.Unlock()
	defer func() {
		a.cancelMu.Lock()
		a.cancel = nil
		a.cancelMu.Unlock()
		cancel()
		goruntime.GC()
		debug.FreeOSMemory()
	}()

	reporter := newRestoreProgressReporter(a.ctx)
	opts := domain.RestoreOptions{
		InputPath:  inputPath,
		KeyPath:    keyPath,
		Password:   password,
		OutputPath: destPath,
	}

	return a.rest.Restore(opCtx, opts, reporter)
}

// SetClipboardText sets plain text to the OS clipboard using the Wails runtime.
func (a *DesktopApp) SetClipboardText(text string) (bool, error) {
	if a.ctx != nil {
		err := runtime.ClipboardSetText(a.ctx, text)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("context not initialized")
}

// GetAuditRecords returns the recent audit records up to limit (newest first).
func (a *DesktopApp) GetAuditRecords(limit int) ([]audit.AuditRecord, error) {
	if a.audit == nil {
		var err error
		a.audit, err = audit.DefaultLogger()
		if err != nil {
			return nil, err
		}
	}
	return a.audit.ReadRecent(limit)
}

// ExportAuditLog exports the entire audit log to destPath.
func (a *DesktopApp) ExportAuditLog(destPath string) error {
	if a.audit == nil {
		var err error
		a.audit, err = audit.DefaultLogger()
		if err != nil {
			return err
		}
	}
	return a.audit.ExportTo(destPath)
}

// ValidateKeyPassword checks password against key security policy requirements.
func (a *DesktopApp) ValidateKeyPassword(password string) domain.PasswordValidationResult {
	trimmed := strings.TrimSpace(password)
	if trimmed == "" {
		// Empty password is valid because it triggers secure auto-generation
		return domain.PasswordValidationResult{
			Valid: true,
			Score: 0,
		}
	}

	score, details := depmap.EvaluatePasswordStrength(password)
	var errs []string
	if !details.OnlyLatin {
		errs = append(errs, depmap.ErrPasswordOnlyLatin.Error())
	}
	if !details.MinLength {
		errs = append(errs, depmap.ErrPasswordTooShort.Error())
	}
	if !details.HasUppercase {
		errs = append(errs, depmap.ErrPasswordNoUppercase.Error())
	}
	if !details.HasLowercase {
		errs = append(errs, depmap.ErrPasswordNoLowercase.Error())
	}
	if !details.HasDigit {
		errs = append(errs, depmap.ErrPasswordNoDigit.Error())
	}
	if !details.HasSpecial {
		errs = append(errs, depmap.ErrPasswordNoSpecial.Error())
	}

	return domain.PasswordValidationResult{
		Valid:        len(errs) == 0,
		Score:        score,
		Errors:       errs,
		MinLength:    details.MinLength,
		HasUppercase: details.HasUppercase,
		HasLowercase: details.HasLowercase,
		HasDigit:     details.HasDigit,
		HasSpecial:   details.HasSpecial,
		OnlyLatin:    details.OnlyLatin,
	}
}

// GenerateArchivePassword generates a strong 16-character random password for archive encryption.
func (a *DesktopApp) GenerateArchivePassword() (string, error) {
	return depmap.GenerateRandomPassword(16)
}

// ValidateArchivePassword validates archive password against basic requirements (ASCII, >=4 chars, no spaces).
func (a *DesktopApp) ValidateArchivePassword(password string) (bool, string) {
	err := ziputil.ValidateArchivePassword(password)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}



