package desktop

import (
	"context"
	goruntime "runtime"
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

// GetTableRows fetches a window of preview rows starting at offset with limit for the default sheet.
func (a *DesktopApp) GetTableRows(path string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error) {
	return a.GetSheetRows(path, "", offset, limit, salt, disabledCols)
}

// GetSheetRows fetches a paginated window of rows for a specific sheet.
func (a *DesktopApp) GetSheetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.prev.GetSheetRows(ctx, path, sheetName, offset, limit, salt, disabledCols)
}

// ProcessFile processes the input file with default output path and options.
func (a *DesktopApp) ProcessFile(path, salt string) (*domain.ProcessResult, error) {
	return a.ProcessFileTo(path, "", salt, nil, false, "", "", false)
}

// ProcessFileTo processes the input file and writes to outPath.
func (a *DesktopApp) ProcessFileTo(
	path, outPath, salt string,
	disabledCols []int,
	createKey bool,
	keyPassword, keyPath string,
	shuffleRows bool,
) (*domain.ProcessResult, error) {
	return a.ProcessFileWithSheets(path, outPath, salt, disabledCols, nil, createKey, keyPassword, keyPath, shuffleRows)
}

// ProcessFileWithSheets processes the input file with per-sheet disabled column options.
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

	reporter := newProcessProgressReporter(a.ctx)
	opts := domain.ProcessOptions{
		InputPath:       path,
		OutputPath:      outPath,
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

