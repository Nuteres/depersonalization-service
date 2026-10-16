package detector

import (
	"strconv"
	"strings"
	"unicode"
)

func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ValidateSNILS(snils string) bool {
	digits := cleanDigits(snils)
	if len(digits) != 11 {
		return false
	}
	num, _ := strconv.ParseInt(digits[:9], 10, 64)
	if num <= 1001998 {
		return true
	}
	sum := 0
	for i := 0; i < 9; i++ {
		d := int(digits[i] - '0')
		sum += d * (9 - i)
	}
	checksum := 0
	if sum < 100 {
		checksum = sum
	} else if sum == 100 || sum == 101 {
		checksum = 0
	} else {
		rem := sum % 101
		if rem < 100 {
			checksum = rem
		}
	}
	expected, _ := strconv.Atoi(digits[9:])
	return checksum == expected
}

func ValidateINN12(inn string) bool {
	digits := cleanDigits(inn)
	if len(digits) != 12 {
		return false
	}
	w11 := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	w12 := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum11 := 0
	for i := 0; i < 10; i++ {
		sum11 += int(digits[i]-'0') * w11[i]
	}
	d11 := (sum11 % 11) % 10
	if int(digits[10]-'0') != d11 {
		return false
	}
	sum12 := 0
	for i := 0; i < 11; i++ {
		sum12 += int(digits[i]-'0') * w12[i]
	}
	d12 := (sum12 % 11) % 10
	return int(digits[11]-'0') == d12
}

// ValidateCardLuhn checks whether a number satisfies the Luhn checksum algorithm (ISO/IEC 7812-1).
func ValidateCardLuhn(card string) bool {
	digits := cleanDigits(card)
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	alternate := false
	for i := len(digits) - 1; i >= 0; i-- {
		n := int(digits[i] - '0')
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

// ValidateCardNumber validates that a string represents a valid bank card:
// 1) Length must be 16, 18, or 19 digits.
// 2) Must start with a known card prefix (Mir, Visa, Mastercard, Maestro, UnionPay, Amex).
// 3) Must pass the Luhn algorithm checksum.
func ValidateCardNumber(card string) bool {
	digits := cleanDigits(card)
	if len(digits) != 16 && len(digits) != 18 && len(digits) != 19 {
		return false
	}
	hasValidPrefix := false
	if strings.HasPrefix(digits, "4") || // Visa
		strings.HasPrefix(digits, "22") || // Mir, MC
		(len(digits) >= 2 && digits[:2] >= "51" && digits[:2] <= "55") || // Mastercard
		(len(digits) >= 2 && (digits[:2] == "50" || digits[:2] == "56" || digits[:2] == "57" || digits[:2] == "58")) || // Maestro
		strings.HasPrefix(digits, "62") || strings.HasPrefix(digits, "67") || strings.HasPrefix(digits, "60") || strings.HasPrefix(digits, "63") || // UnionPay / Maestro
		strings.HasPrefix(digits, "34") || strings.HasPrefix(digits, "37") { // Amex
		hasValidPrefix = true
	}
	if !hasValidPrefix {
		return false
	}
	return ValidateCardLuhn(digits)
}

