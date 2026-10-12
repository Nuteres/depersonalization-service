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
