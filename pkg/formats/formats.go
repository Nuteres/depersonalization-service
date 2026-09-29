package formats

import (
	"context"
	"io"
)

// FormatHandler - единый интерфейс для обработки различных форматов файлов
type FormatHandler interface {
	SupportedExtensions() []string
	Process(ctx context.Context, r io.Reader, w io.Writer, options map[string]interface{}) error
}
