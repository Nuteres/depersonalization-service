package desktop

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dep_service/internal/domain"
)

// emitProcessProgress sends a process-progress event to the Wails runtime.
func emitProcessProgress(ctx context.Context, processed, total, percent int) {
	if ctx != nil && ctx.Value("events") != nil {
		runtime.EventsEmit(ctx, "process-progress", map[string]interface{}{
			"processed": processed,
			"total":     total,
			"percent":   percent,
		})
	}
}

// emitRestoreProgress sends a restore-progress event to the Wails runtime.
func emitRestoreProgress(ctx context.Context, processed, total, percent int) {
	if ctx != nil && ctx.Value("events") != nil {
		runtime.EventsEmit(ctx, "restore-progress", map[string]interface{}{
			"percent":   percent,
			"processed": processed,
			"total":     total,
		})
	}
}

// emitFileDropped sends an app:file-dropped event to the Wails runtime.
func emitFileDropped(ctx context.Context, path string) {
	if ctx != nil && ctx.Value("events") != nil {
		runtime.EventsEmit(ctx, "app:file-dropped", path)
	}
}

// newProcessProgressReporter returns a domain.ProgressReporter that emits "process-progress" events.
func newProcessProgressReporter(ctx context.Context) domain.ProgressReporter {
	return func(processed, total, percent int) {
		emitProcessProgress(ctx, processed, total, percent)
	}
}

// newRestoreProgressReporter returns a domain.ProgressReporter that emits "restore-progress" events.
func newRestoreProgressReporter(ctx context.Context) domain.ProgressReporter {
	return func(processed, total, percent int) {
		emitRestoreProgress(ctx, processed, total, percent)
	}
}
