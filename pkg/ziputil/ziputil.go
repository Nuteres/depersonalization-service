package ziputil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeka/zip"
)

var (
	ErrPasswordTooShort   = errors.New("пароль архива должен содержать не менее 4 символов")
	ErrPasswordInvalidChar = errors.New("пароль архива может содержать только символы латиницы, цифры и спецсимволы без пробелов")
)

// ValidateArchivePassword проверяет пароль архива на соответствие базовым требованиям:
// минимум 4 символа, только печатные символы ASCII (коды 33..126), без пробелов и кириллицы.
func ValidateArchivePassword(password string) error {
	p := strings.TrimSpace(password)
	if len(p) < 4 {
		return ErrPasswordTooShort
	}
	if len(password) != len(p) {
		// Содержит ведущие или замыкающие пробелы
		return ErrPasswordInvalidChar
	}

	for i := 0; i < len(password); i++ {
		b := password[i]
		// Проверяем печатные символы ASCII без пробела (пробел = 32)
		if b < 33 || b > 126 {
			return ErrPasswordInvalidChar
		}
	}
	return nil
}

// CreatePasswordZip упаковывает файл srcFilePath в защищённый паролем ZIP-архив destZipPath,
// используя универсальное шифрование PKWARE (zip.StandardEncryption / ZipCrypto).
// Внутри архива файл сохраняется под своим базовым именем (filepath.Base).
// Создание выполняется атомарно через временный файл .tmp.
func CreatePasswordZip(srcFilePath, destZipPath, password string) (err error) {
	if err := ValidateArchivePassword(password); err != nil {
		return err
	}

	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return fmt.Errorf("не удалось открыть исходный файл для архивации: %w", err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(destZipPath), 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию архива: %w", err)
	}

	tmpZipPath := destZipPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpZipPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл архива: %w", err)
	}

	cleanedUp := false
	cleanup := func() {
		if !cleanedUp {
			cleanedUp = true
			_ = tmpFile.Close()
			_ = os.Remove(tmpZipPath)
		}
	}
	defer cleanup()

	zipWriter := zip.NewWriter(tmpFile)

	entryName := filepath.Base(srcFilePath)
	entryWriter, err := zipWriter.Encrypt(entryName, password, zip.StandardEncryption)
	if err != nil {
		return fmt.Errorf("не удалось инициализировать шифрование архива: %w", err)
	}

	if _, err := io.Copy(entryWriter, srcFile); err != nil {
		return fmt.Errorf("ошибка при записи файла в архив: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("не удалось финализировать архив: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("не удалось закрыть временный файл архива: %w", err)
	}

	// Переименовываем временный файл в целевой архив
	if err := os.Rename(tmpZipPath, destZipPath); err != nil {
		return fmt.Errorf("не удалось сохранить итоговый архив: %w", err)
	}

	cleanedUp = true
	return nil
}
