package engine

import (
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

// Engine - оркестратор потоковой обработки файлов
type Engine struct {
	handlers   map[string]formats.FormatHandler
	anonymizer *anonymizer.Anonymizer
	detectors  []detector.EntityType
}

func NewEngine(salt string) *Engine {
	return &Engine{
		handlers:   make(map[string]formats.FormatHandler),
		anonymizer: anonymizer.New(salt),
	}
}
