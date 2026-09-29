package anonymizer

import (
	"fmt"
	"strings"
	"testing"
)

func validateLuhn(number string) bool {
	sum := 0
	alternate := false
	for i := len(number) - 1; i >= 0; i-- {
		n := int(number[i] - '0')
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}

func TestMIRCardLuhn(t *testing.T) {
	anon := New("salt-finance")
	card := anon.GenerateCardMIR("4111222233334444")
	clean := strings.ReplaceAll(card, " ", "")
	if !strings.HasPrefix(clean, "2200") {
		t.Fatalf("expected MIR prefix 2200, got %s", clean)
	}
	if len(clean) != 16 {
		t.Fatalf("expected 16 digits, got %d in %s", len(clean), clean)
	}
	if !validateLuhn(clean) {
		t.Fatalf("MIR card %s failed Luhn check", clean)
	}
}

func TestMIRCardDeterminism(t *testing.T) {
	anon1 := New("salt-finance-1")
	anon2 := New("salt-finance-2")

	orig := "4111222233334444"
	card1 := anon1.GenerateCardMIR(orig)
	card2 := anon1.GenerateCardMIR(orig)
	card3 := anon2.GenerateCardMIR(orig)
	card4 := anon1.GenerateCardMIR("5555444433332222")

	if card1 != card2 {
		t.Fatalf("expected deterministic output: %s != %s", card1, card2)
	}
	if card1 == card3 {
		t.Fatalf("expected different salts to produce different cards: %s == %s", card1, card3)
	}
	if card1 == card4 {
		t.Fatalf("expected different originals to produce different cards: %s == %s", card1, card4)
	}
}

func TestMIRCardMultipleGenerations(t *testing.T) {
	anon := New("salt-finance-batch")
	for i := 0; i < 50; i++ {
		orig := fmt.Sprintf("card-%d", i)
		card := anon.GenerateCardMIR(orig)
		clean := strings.ReplaceAll(card, " ", "")
		if !strings.HasPrefix(clean, "2200") {
			t.Fatalf("expected MIR prefix 2200 for %s, got %s", orig, clean)
		}
		if len(clean) != 16 {
			t.Fatalf("expected 16 digits for %s, got %d (%s)", orig, len(clean), clean)
		}
		if !validateLuhn(clean) {
			t.Fatalf("MIR card %s failed Luhn check", clean)
		}
	}
}
