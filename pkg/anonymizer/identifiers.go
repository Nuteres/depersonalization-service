package anonymizer

import (
	"fmt"
)

func (a *Anonymizer) GenerateSNILS(original string) string {
	seed := a.DeriveSeed(original)
	n1 := a.RandInt(seed, 100, 999)
	n2 := a.RandInt(seed+1, 100, 999)
	n3 := a.RandInt(seed+2, 100, 999)
	digitsStr := fmt.Sprintf("%03d%03d%03d", n1, n2, n3)

	sum := 0
	for i := 0; i < 9; i++ {
		d := int(digitsStr[i] - '0')
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
	return fmt.Sprintf("%03d-%03d-%03d %02d", n1, n2, n3, checksum)
}

func (a *Anonymizer) GenerateINN12(original string) string {
	seed := a.DeriveSeed(original)
	region := a.RandInt(seed, 10, 99)
	inspect := a.RandInt(seed+1, 10, 99)
	seq := a.RandInt(seed+2, 100000, 999999)
	base := fmt.Sprintf("%02d%02d%06d", region, inspect, seq)

	w11 := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum11 := 0
	for i := 0; i < 10; i++ {
		sum11 += int(base[i]-'0') * w11[i]
	}
	d11 := (sum11 % 11) % 10

	w12 := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	baseWith11 := fmt.Sprintf("%s%d", base, d11)
	sum12 := 0
	for i := 0; i < 11; i++ {
		sum12 += int(baseWith11[i]-'0') * w12[i]
	}
	d12 := (sum12 % 11) % 10
	return fmt.Sprintf("%s%d%d", base, d11, d12)
}
