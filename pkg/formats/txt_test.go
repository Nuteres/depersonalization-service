package formats

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

func TestTXTHandler_Process_Tokens(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewTXTHandler(anon)

	input := "Сотрудник Иванов Иван Иванович имеет паспорт 4508 123456 и телефон +7 916 123-45-67."
	dm := depmap.New("test.txt")
	opts := map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"salt":     "salt",
		"depmap":   dm,
	}

	var out bytes.Buffer
	err := h.Process(context.Background(), strings.NewReader(input), &out, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	res := out.String()
	if !strings.Contains(res, "[ФИО_0001]") {
		t.Fatalf("expected [ФИО_0001] in output, got: %s", res)
	}
	if !strings.Contains(res, "[Паспорт_0001]") {
		t.Fatalf("expected [Паспорт_0001] in output, got: %s", res)
	}
	if dm.RecordsCount < 2 {
		t.Fatalf("expected at least 2 records in depmap, got %d", dm.RecordsCount)
	}
}

func TestRestoreTXT(t *testing.T) {
	dm := depmap.New("test.txt")
	dm.Put("[ФИО_0001]", "Иванов Иван Иванович")
	dm.Put("[Паспорт_0001]", "4508 123456")

	input := "Вывод нейросети: Данные принадлежат [ФИО_0001], документ [Паспорт_0001]."
	var out bytes.Buffer

	count, err := RestoreTXT(context.Background(), strings.NewReader(input), &out, dm)
	if err != nil {
		t.Fatalf("RestoreTXT failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 replacements, got %d", count)
	}

	expected := "Вывод нейросети: Данные принадлежат Иванов Иван Иванович, документ 4508 123456."
	if out.String() != expected {
		t.Errorf("expected %q, got %q", expected, out.String())
	}
}

func TestTXTHandler_Inspect(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewTXTHandler(anon)

	input := "Сотрудник Иванов Иван Иванович имеет паспорт 4508 123456.\nВторая строка без данных."
	lineCount, entitiesCounts, snippets, paragraphPreviews, documentBlocks, err := h.Inspect(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if lineCount != 2 {
		t.Errorf("expected 2 lines, got %d", lineCount)
	}
	if len(entitiesCounts) < 2 {
		t.Errorf("expected at least 2 entity types, got %d", len(entitiesCounts))
	}
	if len(snippets) < 2 {
		t.Errorf("expected at least 2 snippets, got %d", len(snippets))
	}
	if len(paragraphPreviews) != 2 {
		t.Errorf("expected 2 paragraph previews, got %d", len(paragraphPreviews))
	}
	if len(documentBlocks) != 2 {
		t.Errorf("expected 2 document blocks, got %d", len(documentBlocks))
	}
}


