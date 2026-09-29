package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"dep_service/frontend"
)

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Деперсонализация ПД (152-ФЗ)",
		Width:  1200,
		Height: 850,
		AssetServer: &assetserver.Options{
			Assets: frontend.Assets,
		},
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Ошибка запуска Wails приложения: %v", err)
	}
}
