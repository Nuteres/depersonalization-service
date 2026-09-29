# План реализации: Приложение автоматической деперсонализации данных и файлов (152-ФЗ)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Создать кроссплатформенное десктопное приложение на Go (Wails) в виде однофайлового бинарника для автоматического обнаружения и обезличивания ПДн в выгрузках (Excel, CSV, JSON) и документах (PDF) по 152-ФЗ с нативной поддержкой Alt Linux 10.4, Windows и macOS.

**Architecture:** Модульное ядро на Go с разделением на детекторы сущностей (`pkg/detector`), детерминированные синтетические генераторы (`pkg/anonymizer`), потоковые обработчики форматов (`pkg/formats`) и оркестратор (`pkg/engine`). Графический интерфейс на Wails с нативным WebView и локальным IPC-мостом без сетевых запросов.

**Tech Stack:** Go 1.22+, Wails v2/v3, excelize/v2 (Excel streaming), encoding/csv, PDF parser/redactor, HTML5/CSS (Plus Jakarta Sans + JetBrains Mono), Docker (`alt:p10`).

**Spec:** `docs/superpowers/specs/2026-09-29-data-depersonalizer-design.md`

## Global Constraints

- **On-Premise / Air-Gapped**: Запрещены любые внешние HTTP-запросы, трекеры и CDN. Все шрифты, стили и словари вшиваются внутрь бинарника (`//go:embed`).
- **Alt Linux 10.4 Compatibility**: Сборка под Linux выполняется строго в контейнере `registry.altlinux.org/alt/p10` против `glibc 2.32`.
- **Сохранение связей**: Детерминированная замена `HMAC-SHA256(value, salt)` гарантирует, что одинаковые исходные ПДн везде заменяются на одинаковые синтетические данные.
- **TDD (Test-Driven Development)**: Каждый модуль сопровождается unit-тестами с проверкой контрольных сумм и детерминированности.

---

### Task 1: Детерминированный генератор и управление солью

**Files:**
- Create: `pkg/anonymizer/salt.go`
- Modify: `pkg/anonymizer/anonymizer.go`
- Test: `pkg/anonymizer/anonymizer_test.go`

**Interfaces:**
- Produces:
  ```go
  type Anonymizer struct { ... }
  func New(salt string) *Anonymizer
  func (a *Anonymizer) DeriveSeed(val string) uint64
  func (a *Anonymizer) RandInt(seed uint64, min, max int) int
  ```

- [ ] **Step 1: Write the failing test**

```go
// pkg/anonymizer/anonymizer_test.go
package anonymizer

import "testing"

func TestDeterministicSeed(t *testing.T) {
	anon := New("secret-salt-152fz")
	seed1 := anon.DeriveSeed("Иванов Иван Иванович")
	seed2 := anon.DeriveSeed("Иванов Иван Иванович")
	seed3 := anon.DeriveSeed("Петров Петр Петрович")

	if seed1 != seed2 {
		t.Fatalf("expected identical seed for same input, got %d and %d", seed1, seed2)
	}
	if seed1 == seed3 {
		t.Fatalf("expected different seed for different input, got %d", seed1)
	}
}

func TestRandIntRange(t *testing.T) {
	anon := New("salt")
	seed := anon.DeriveSeed("test")
	val := anon.RandInt(seed, 10, 20)
	if val < 10 || val > 20 {
		t.Fatalf("RandInt out of range: %d", val)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/anonymizer`
Expected: FAIL with undefined `RandInt`.

- [ ] **Step 3: Write minimal implementation**

```go
// pkg/anonymizer/anonymizer.go
package anonymizer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

type Anonymizer struct {
	salt []byte
}

func New(salt string) *Anonymizer {
	return &Anonymizer{salt: []byte(salt)}
}

func (a *Anonymizer) DeriveSeed(originalValue string) uint64 {
	mac := hmac.New(sha256.New, a.salt)
	mac.Write([]byte(originalValue))
	sum := mac.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

func (a *Anonymizer) RandInt(seed uint64, min, max int) int {
	if min >= max {
		return min
	}
	// Linear Congruential Generator step
	next := seed*6364136223846793005 + 1442695040888963407
	diff := uint64(max - min + 1)
	return min + int(next%diff)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/anonymizer`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/anonymizer/
git commit -m "feat(anonymizer): implement deterministic seed derivation and random bounds"
```

---

### Task 2: Валидаторы и генераторы СНИЛС и ИНН с контрольными суммами РФ

**Files:**
- Create: `pkg/detector/checksums.go`
- Create: `pkg/anonymizer/identifiers.go`
- Test: `pkg/detector/checksums_test.go`
- Test: `pkg/anonymizer/identifiers_test.go`

**Interfaces:**
- Consumes: `Anonymizer.DeriveSeed`, `Anonymizer.RandInt`
- Produces:
  ```go
  func ValidateSNILS(snils string) bool
  func ValidateINN12(inn string) bool
  func (a *Anonymizer) GenerateSNILS(original string) string
  func (a *Anonymizer) GenerateINN12(original string) string
  ```

- [ ] **Step 1: Write the failing tests**

```go
// pkg/detector/checksums_test.go
package detector

import "testing"

func TestSNILSValidation(t *testing.T) {
	// Valid SNILS with correct modulo 101 checksum
	validSNILS := "112-233-445 95"
	if !ValidateSNILS(validSNILS) {
		t.Fatalf("expected %s to be valid", validSNILS)
	}
	invalidSNILS := "112-233-445 00"
	if ValidateSNILS(invalidSNILS) {
		t.Fatalf("expected %s to be invalid", invalidSNILS)
	}
}

func TestINN12Validation(t *testing.T) {
	// Valid 12-digit Russian personal INN with official weights
	validINN := "500100732259"
	if !ValidateINN12(validINN) {
		t.Fatalf("expected %s to be valid INN-12", validINN)
	}
	invalidINN := "500100732250"
	if ValidateINN12(invalidINN) {
		t.Fatalf("expected %s to be invalid INN-12", invalidINN)
	}
}
```

```go
// pkg/anonymizer/identifiers_test.go
package anonymizer

import (
	"dep_service/pkg/detector"
	"testing"
)

func TestGenerateValidSNILS(t *testing.T) {
	anon := New("salt-152fz")
	generated := anon.GenerateSNILS("112-233-445 95")
	if !detector.ValidateSNILS(generated) {
		t.Fatalf("generated SNILS %s failed modulo 101 validation", generated)
	}
}

func TestGenerateValidINN12(t *testing.T) {
	anon := New("salt-152fz")
	generated := anon.GenerateINN12("500100732259")
	if !detector.ValidateINN12(generated) {
		t.Fatalf("generated INN %s failed Russian FNS checksums", generated)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/detector ./pkg/anonymizer`
Expected: FAIL with compilation errors (missing functions).

- [ ] **Step 3: Write implementation**

```go
// pkg/detector/checksums.go
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
```

```go
// pkg/anonymizer/identifiers.go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/detector ./pkg/anonymizer`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/detector/ pkg/anonymizer/
git commit -m "feat(identifiers): implement SNILS and INN-12 validators and deterministic generators"
```

---

### Task 3: Генерация русских ФИО с сохранением пола

**Files:**
- Create: `pkg/anonymizer/data/names_ru.go`
- Create: `pkg/anonymizer/fio.go`
- Test: `pkg/anonymizer/fio_test.go`

**Interfaces:**
- Consumes: `Anonymizer.DeriveSeed`, `Anonymizer.RandInt`
- Produces:
  ```go
  type Gender int
  const (GenderMale Gender = 0; GenderFemale Gender = 1)
  func DetectGender(fio string) Gender
  func (a *Anonymizer) GenerateFIO(original string) string
  ```

- [ ] **Step 1: Write the failing test**

```go
// pkg/anonymizer/fio_test.go
package anonymizer

import "testing"

func TestGenderPreservation(t *testing.T) {
	anon := New("salt-fio")
	maleOriginal := "Иванов Иван Иванович"
	maleGen := anon.GenerateFIO(maleOriginal)
	if DetectGender(maleGen) != GenderMale {
		t.Fatalf("expected male replacement for %s, got %s", maleOriginal, maleGen)
	}

	femaleOriginal := "Смирнова Ольга Алексеевна"
	femaleGen := anon.GenerateFIO(femaleOriginal)
	if DetectGender(femaleGen) != GenderFemale {
		t.Fatalf("expected female replacement for %s, got %s", femaleOriginal, femaleGen)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/anonymizer -run TestGenderPreservation`
Expected: FAIL.

- [ ] **Step 3: Write implementation**

Create embedded dictionary lists in `pkg/anonymizer/data/names_ru.go` and gender logic in `pkg/anonymizer/fio.go`.

```go
// pkg/anonymizer/fio.go
package anonymizer

import (
	"fmt"
	"strings"
)

type Gender int

const (
	GenderMale Gender = iota
	GenderFemale
)

func DetectGender(fio string) Gender {
	parts := strings.Fields(fio)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, "овна") || strings.HasSuffix(lower, "евна") || strings.HasSuffix(lower, "ична") || strings.HasSuffix(lower, "ая") || strings.HasSuffix(lower, "ова") || strings.HasSuffix(lower, "ева") {
			return GenderFemale
		}
	}
	return GenderMale
}

var MaleSurnames = []string{"Смирнов", "Иванов", "Кузнецов", "Попов", "Соколов", "Лебедев", "Козлов", "Новиков", "Морозов", "Петров", "Волков", "Соловьев", "Васильев", "Зайцев", "Павлов"}
var FemaleSurnames = []string{"Смирнова", "Иванова", "Кузнецова", "Попова", "Соколова", "Лебедева", "Козлова", "Новикова", "Морозова", "Петрова", "Волкова", "Соловьева", "Васильева", "Зайцева", "Павлова"}

var MaleNames = []string{"Александр", "Михаил", "Максим", "Артем", "Даниил", "Иван", "Дмитрий", "Кирилл", "Андрей", "Егор", "Роман", "Сергей", "Никита"}
var FemaleNames = []string{"София", "Анна", "Мария", "Алиса", "Ева", "Виктория", "Полина", "Екатерина", "Ксения", "Дарья", "Анастасия", "Ольга"}

var MalePatronymics = []string{"Александрович", "Михайлович", "Максимович", "Иванович", "Дмитриевич", "Андреевич", "Сергеевич", "Романович", "Павлович"}
var FemalePatronymics = []string{"Александровна", "Михайловна", "Максимовна", "Ивановна", "Дмитриевна", "Андреевна", "Сергеевна", "Романовна", "Павловна"}

func (a *Anonymizer) GenerateFIO(original string) string {
	seed := a.DeriveSeed(original)
	gender := DetectGender(original)

	if gender == GenderFemale {
		sIdx := a.RandInt(seed, 0, len(FemaleSurnames)-1)
		nIdx := a.RandInt(seed+1, 0, len(FemaleNames)-1)
		pIdx := a.RandInt(seed+2, 0, len(FemalePatronymics)-1)
		return fmt.Sprintf("%s %s %s", FemaleSurnames[sIdx], FemaleNames[nIdx], FemalePatronymics[pIdx])
	}
	sIdx := a.RandInt(seed, 0, len(MaleSurnames)-1)
	nIdx := a.RandInt(seed+1, 0, len(MaleNames)-1)
	pIdx := a.RandInt(seed+2, 0, len(MalePatronymics)-1)
	return fmt.Sprintf("%s %s %s", MaleSurnames[sIdx], MaleNames[nIdx], MalePatronymics[pIdx])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/anonymizer -run TestGenderPreservation`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/anonymizer/
git commit -m "feat(fio): implement Russian gender detection and authentic name generation"
```

---

### Task 4: Документы, контакты, карты (МИР/Луна) и даты со сдвигом

**Files:**
- Create: `pkg/anonymizer/documents.go`
- Create: `pkg/anonymizer/finance.go`
- Test: `pkg/anonymizer/documents_test.go`
- Test: `pkg/anonymizer/finance_test.go`

**Interfaces:**
- Produces:
  ```go
  func (a *Anonymizer) GeneratePassport(original string) string
  func (a *Anonymizer) GeneratePhone(original string) string
  func (a *Anonymizer) GenerateCardMIR(original string) string
  func (a *Anonymizer) PerturbDate(originalDate string, daysRange int) string
  ```

- [ ] **Step 1: Write failing tests for Luhn and Passport**

```go
// pkg/anonymizer/finance_test.go
package anonymizer

import (
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
	if !validateLuhn(clean) {
		t.Fatalf("MIR card %s failed Luhn check", clean)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/anonymizer -run TestMIRCardLuhn`
Expected: FAIL.

- [ ] **Step 3: Implement documents, contacts and finance generators**

Implement `GeneratePassport`, `GeneratePhone`, `GenerateCardMIR`, `PerturbDate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/anonymizer`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/anonymizer/
git commit -m "feat(finance): implement MIR card with Luhn, passport, phone and date perturbation"
```

---

### Task 5: Потоковые обработчики Excel (XLSX) и CSV

**Files:**
- Create: `pkg/formats/csv_handler.go`
- Create: `pkg/formats/excel_handler.go`
- Test: `pkg/formats/csv_test.go`
- Test: `pkg/formats/excel_test.go`

**Interfaces:**
- Consumes: `pkg/formats.FormatHandler`, `pkg/anonymizer.Anonymizer`
- Produces:
  ```go
  type CSVHandler struct { Anon *anonymizer.Anonymizer }
  type ExcelHandler struct { Anon *anonymizer.Anonymizer }
  ```

- [ ] **Step 1: Write test for CSV and Excel streaming**
- [ ] **Step 2: Run test to verify failure**
- [ ] **Step 3: Implement streaming with bufio.Reader and excelize StreamWriter**
- [ ] **Step 4: Run tests to verify passing**
- [ ] **Step 5: Commit**

```bash
git add pkg/formats/ go.mod go.sum
git commit -m "feat(formats): implement streaming CSV and Excel XLSX anonymizers"
```

---

### Task 6: PDF Blackout Redaction Engine

**Files:**
- Create: `pkg/formats/pdf_handler.go`
- Test: `pkg/formats/pdf_test.go`

**Interfaces:**
- Scans sensitive text in PDF pages, finds bounding boxes, scrubs text, draws solid black boxes.

- [ ] **Step 1: Write test with sample text in PDF**
- [ ] **Step 2: Verify test fails**
- [ ] **Step 3: Implement PDF stream editor with blackout rectangle overlay**
- [ ] **Step 4: Run test to verify redacted text cannot be extracted**
- [ ] **Step 5: Commit**

```bash
git add pkg/formats/pdf_handler.go pkg/formats/pdf_test.go
git commit -m "feat(pdf): implement blackout redaction with physical text scrubbing"
```

---

### Task 7: Интеграция Wails GUI и фронтенда

**Files:**
- Create: `cmd/app/wails_app.go`
- Modify: `cmd/app/main.go`
- Link: `frontend/index.html` via `//go:embed`

**Interfaces:**
- Wails bindings:
  ```go
  func (a *App) InspectFile(path string) (*FileInfo, error)
  func (a *App) ProcessFile(path string, salt string) (*ProcessResult, error)
  ```

- [ ] **Step 1: Create Wails App bindings and IPC handlers**
- [ ] **Step 2: Connect frontend JavaScript to Go IPC functions**
- [ ] **Step 3: Test local compilation of Wails executable**
- [ ] **Step 4: Commit**

```bash
git add cmd/app/ frontend/
git commit -m "feat(gui): integrate Wails IPC with light-theme frontend"
```

---

### Task 8: Сборка под Alt Linux 10.4 (Docker) и кроссплатформенный Makefile

**Files:**
- Test: `make build-altlinux`
- Output: `bin/depersonalizer-altlinux-amd64`

- [ ] **Step 1: Run Docker build test against alt:p10**
- [ ] **Step 2: Verify ELF binary glibc requirements using `objdump -p`**
- [ ] **Step 3: Verify single-file standalone execution**
- [ ] **Step 4: Commit**

```bash
git add Makefile build/altlinux/
git commit -m "ci(build): verify Alt Linux 10.4 containerized build pipeline"
```
