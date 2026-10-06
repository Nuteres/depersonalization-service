package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SelectFile opens a native system file selection dialog and returns the selected path.
func (a *DesktopApp) SelectFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	var filters []runtime.FileFilter
	if goruntime.GOOS != "darwin" {
		filters = []runtime.FileFilter{
			{
				DisplayName: "Все поддерживаемые форматы (*.xlsx, *.xlsb, *.ods, *.xls, *.csv, *.docx, *.odt, *.pdf)",
				Pattern:     "*.xlsx;*.xlsb;*.ods;*.xls;*.csv;*.tsv;*.docx;*.odt;*.pdf",
			},
			{
				DisplayName: "Таблицы Excel и ODS (*.xlsx, *.xlsb, *.ods, *.xls, *.csv)",
				Pattern:     "*.xlsx;*.xlsb;*.ods;*.xls;*.csv;*.tsv",
			},
			{
				DisplayName: "Текстовые документы Word и ODT (*.docx, *.odt)",
				Pattern:     "*.docx;*.odt",
			},
			{
				DisplayName: "Документы PDF (*.pdf)",
				Pattern:     "*.pdf",
			},
		}
	}

	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Выберите файл для обезличивания",
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	return selected, nil
}

// SaveFileDialog opens a native system file save dialog with defaultFilename
// and returns the user-selected destination path.
func (a *DesktopApp) SaveFileDialog(defaultName string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	var filters []runtime.FileFilter
	if goruntime.GOOS != "darwin" {
		ext := strings.ToLower(filepath.Ext(defaultName))
		switch ext {
		case ".xlsx", ".xlsm", ".xlsb", ".xls":
			filters = []runtime.FileFilter{
				{DisplayName: "Таблица Excel (*.xlsx)", Pattern: "*.xlsx"},
			}
		case ".ods":
			filters = []runtime.FileFilter{
				{DisplayName: "Таблица OpenDocument (*.ods)", Pattern: "*.ods"},
				{DisplayName: "Таблица Excel (*.xlsx)", Pattern: "*.xlsx"},
			}
		case ".docx":
			filters = []runtime.FileFilter{
				{DisplayName: "Документ Word (*.docx)", Pattern: "*.docx"},
				{DisplayName: "Документ OpenDocument Text (*.odt)", Pattern: "*.odt"},
			}
		case ".odt":
			filters = []runtime.FileFilter{
				{DisplayName: "Документ OpenDocument Text (*.odt)", Pattern: "*.odt"},
				{DisplayName: "Документ Word (*.docx)", Pattern: "*.docx"},
			}
		case ".pdf":
			filters = []runtime.FileFilter{
				{DisplayName: "Документ PDF (*.pdf)", Pattern: "*.pdf"},
			}
		default:
			filters = []runtime.FileFilter{
				{DisplayName: "Таблица CSV (*.csv)", Pattern: "*.csv"},
				{DisplayName: "Таблица Excel (*.xlsx)", Pattern: "*.xlsx"},
			}
		}
	}

	selected, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить обработанный файл",
		DefaultFilename: defaultName,
		Filters:         filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open save dialog: %w", err)
	}
	return selected, nil
}

// SelectKeyFile opens a native system file selection dialog filtering for .depmap key files.
func (a *DesktopApp) SelectKeyFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	var filters []runtime.FileFilter
	if goruntime.GOOS != "darwin" {
		filters = []runtime.FileFilter{
			{
				DisplayName: "Файлы-ключи восстановления (*.depmap)",
				Pattern:     "*.depmap",
			},
		}
	}

	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Выберите файл-ключ восстановления (.depmap)",
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	return selected, nil
}

// SaveKeyFileDialog opens a native system file save dialog for .depmap key files.
func (a *DesktopApp) SaveKeyFileDialog(defaultName string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	var filters []runtime.FileFilter
	if goruntime.GOOS != "darwin" {
		filters = []runtime.FileFilter{
			{DisplayName: "Файл-ключ восстановления (*.depmap)", Pattern: "*.depmap"},
		}
	}

	selected, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить ключ восстановления (.depmap)",
		DefaultFilename: defaultName,
		Filters:         filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open save dialog: %w", err)
	}
	return selected, nil
}

// OpenContainingFolder opens the folder containing targetPath in the OS file manager.
func (a *DesktopApp) OpenContainingFolder(targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return errors.New("path cannot be empty")
	}

	dir := filepath.Dir(targetPath)
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", targetPath).Start()
	case "windows":
		return exec.Command("explorer.exe", "/select,", targetPath).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

func detectOS() string {
	switch goruntime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		if data, err := os.ReadFile("/etc/altlinux-release"); err == nil && len(data) > 0 {
			return "Alt Linux"
		}
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			s := string(data)
			if strings.Contains(s, "ALT") || strings.Contains(s, "altlinux") {
				return "Alt Linux"
			}
			for _, line := range strings.Split(s, "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					val := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					if val != "" {
						return val
					}
				}
			}
		}
		return "Linux"
	default:
		return goruntime.GOOS
	}
}
