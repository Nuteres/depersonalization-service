package anonymizer

import (
	"fmt"
)

// GenerateCardMIR генерирует синтетический номер банковской карты национальной платежной системы МИР.
// Номер начинается с префикса 2200, состоит из 16 цифр (4 блока по 4 цифры) и проходит валидацию
// по контрольной сумме алгоритма Луна (Luhn algorithm).
func (a *Anonymizer) GenerateCardMIR(original string) string {
	seed := a.DeriveSeed(original)

	p1 := a.RandInt(seed, 0, 9999)
	p2 := a.RandInt(seed+1, 0, 9999)
	p3 := a.RandInt(seed+2, 0, 999)

	first15 := fmt.Sprintf("2200%04d%04d%03d", p1, p2, p3)

	sum := 0
	for i := 0; i < 15; i++ {
		d := int(first15[i] - '0')
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}

	checkDigit := (10 - (sum % 10)) % 10

	return fmt.Sprintf("2200 %04d %04d %03d%d", p1, p2, p3, checkDigit)
}
