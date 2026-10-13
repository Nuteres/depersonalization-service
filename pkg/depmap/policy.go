package depmap

import (
	"errors"
	"strings"
)

var (
	// ErrPasswordTooShort is returned when password has fewer than 12 characters.
	ErrPasswordTooShort = errors.New("пароль должен содержать не менее 12 символов")
	// ErrPasswordNoUppercase is returned when password contains no Latin uppercase letters.
	ErrPasswordNoUppercase = errors.New("пароль должен содержать хотя бы одну заглавную латинскую букву (A-Z)")
	// ErrPasswordNoLowercase is returned when password contains no Latin lowercase letters.
	ErrPasswordNoLowercase = errors.New("пароль должен содержать хотя бы одну строчную латинскую букву (a-z)")
	// ErrPasswordNoDigit is returned when password contains no digits.
	ErrPasswordNoDigit = errors.New("пароль должен содержать хотя бы одну цифру (0-9)")
	// ErrPasswordNoSpecial is returned when password contains no special characters.
	ErrPasswordNoSpecial = errors.New("пароль должен содержать хотя бы один специальный символ")
	// ErrPasswordOnlyLatin is returned when password contains non-Latin or unsupported characters.
	ErrPasswordOnlyLatin = errors.New("пароль должен содержать только латинские буквы (A-Z, a-z), цифры и спецсимволы")
)

// PasswordPolicyDetails represents individual criterion completion for a password.
type PasswordPolicyDetails struct {
	MinLength    bool `json:"minLength"`
	HasUppercase bool `json:"hasUppercase"`
	HasLowercase bool `json:"hasLowercase"`
	HasDigit     bool `json:"hasDigit"`
	HasSpecial   bool `json:"hasSpecial"`
	OnlyLatin    bool `json:"onlyLatin"`
}

func isLatinUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isLatinLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isSpecialChar(r rune) bool {
	return strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?/\\~`'\"", r)
}

// CheckPasswordCriteria evaluates which requirements the password satisfies.
func CheckPasswordCriteria(password string) PasswordPolicyDetails {
	runes := []rune(password)
	details := PasswordPolicyDetails{
		MinLength: len(runes) >= 12,
		OnlyLatin: len(runes) > 0,
	}

	for _, r := range runes {
		isUpper := isLatinUpper(r)
		isLower := isLatinLower(r)
		isDig := isDigit(r)
		isSpec := isSpecialChar(r)

		if isUpper {
			details.HasUppercase = true
		}
		if isLower {
			details.HasLowercase = true
		}
		if isDig {
			details.HasDigit = true
		}
		if isSpec {
			details.HasSpecial = true
		}

		if !isUpper && !isLower && !isDig && !isSpec {
			details.OnlyLatin = false
		}
	}

	return details
}

// ValidatePassword validates password against the key file security policy:
// minimum 12 characters, Latin only (A-Z, a-z, 0-9, special characters),
// with at least one uppercase letter, one lowercase letter, one digit, and one special symbol.
func ValidatePassword(password string) error {
	trimmed := strings.TrimSpace(password)
	if trimmed == "" {
		return ErrEmptyPassword
	}

	details := CheckPasswordCriteria(password)
	if !details.OnlyLatin {
		return ErrPasswordOnlyLatin
	}
	if !details.MinLength {
		return ErrPasswordTooShort
	}
	if !details.HasUppercase {
		return ErrPasswordNoUppercase
	}
	if !details.HasLowercase {
		return ErrPasswordNoLowercase
	}
	if !details.HasDigit {
		return ErrPasswordNoDigit
	}
	if !details.HasSpecial {
		return ErrPasswordNoSpecial
	}

	return nil
}

// EvaluatePasswordStrength returns a strength score from 0 to 4 along with criterion details.
// 0 = empty or whitespace
// 1 = short or very weak / contains non-Latin characters
// 2 = medium (meets 2 criteria)
// 3 = good (meets 3-4 criteria, but maybe short or missing one)
// 4 = strong (satisfies all policy criteria: >=12 chars + 4 character types + Latin only)
func EvaluatePasswordStrength(password string) (int, PasswordPolicyDetails) {
	if strings.TrimSpace(password) == "" {
		return 0, PasswordPolicyDetails{}
	}

	details := CheckPasswordCriteria(password)
	if !details.OnlyLatin {
		return 1, details
	}

	criteriaCount := 0
	if details.HasUppercase {
		criteriaCount++
	}
	if details.HasLowercase {
		criteriaCount++
	}
	if details.HasDigit {
		criteriaCount++
	}
	if details.HasSpecial {
		criteriaCount++
	}

	if details.MinLength && criteriaCount == 4 {
		return 4, details
	}

	if details.MinLength && criteriaCount >= 3 {
		return 3, details
	}

	if criteriaCount >= 3 || (details.MinLength && criteriaCount >= 2) {
		return 2, details
	}

	return 1, details
}
