package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"dep_service/frontend"
	"dep_service/internal/desktop"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
)

func main() {
	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km, insp)
	rest := restorer.New(km)
	app := desktop.New(insp, prev, proc, rest, km)

	err := wails.Run(&options.App{
		Title:  "Деперсонализация ПД (152-ФЗ)",
		Width:  1200,
		Height: 850,
		AssetServer: &assetserver.Options{
			Assets: frontend.Assets,
		},
		OnStartup: app.Startup,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Ошибка запуска Wails приложения: %v", err)
	}
}
