package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"dep_service/internal/version"
)

func main() {
	syncNotion := flag.Bool("notion", false, "Выполнить отправку и синхронизацию с Notion API")
	flag.Parse()

	fmt.Printf("=== Утилита синхронизации базы знаний и FAQ (Версия %s) ===\n", version.Version)

	scriptPath := filepath.Join("tools", "sync_kb", "sync_to_notion.py")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		fmt.Printf("Ошибка: файл скрипта %s не найден\n", scriptPath)
		os.Exit(1)
	}

	if *syncNotion {
		fmt.Println("Запуск синхронизации базы знаний с Notion...")
		cmd := exec.Command("python3", scriptPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Ошибка при выполнении синхронизации: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Режим проверки: для отправки в Notion передайте флаг -notion")
		fmt.Printf("Локальная реплика базы знаний: docs/KNOWLEDGE_BASE.md (версия v%s)\n", version.Version)
	}
}
