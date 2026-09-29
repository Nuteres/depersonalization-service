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
