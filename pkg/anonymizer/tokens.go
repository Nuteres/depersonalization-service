package anonymizer

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// TokenType represents a classified personal data category for semantic tokens.
type TokenType string

const (
	TokenFIO        TokenType = "ФИО"
	TokenLastName   TokenType = "Фамилия"
	TokenFirstName  TokenType = "Имя"
	TokenPatronymic TokenType = "Отчество"
	TokenPassport   TokenType = "Паспорт"
	TokenSNILS      TokenType = "СНИЛС"
	TokenINN        TokenType = "ИНН"
	TokenPhone      TokenType = "Тел"
	TokenEmail      TokenType = "Email"
	TokenAddress    TokenType = "Адрес"
	TokenCard       TokenType = "Карта"
	TokenDate       TokenType = "Дата"
	TokenEmployeeID TokenType = "Табель"
	TokenGrade      TokenType = "Грейд"
)

var reBracketedToken = regexp.MustCompile(`^\[[А-Яа-яA-Za-z0-9_]+\]$`)

// IsBracketedToken returns true if the string matches the semantic token pattern [Type_XXXX].
func IsBracketedToken(val string) bool {
	return reBracketedToken.MatchString(strings.TrimSpace(val))
}

// TokenSession manages deterministic, sequence-based semantic tokens for a de-identification run.
type TokenSession struct {
	salt     string
	mu       sync.Mutex
	counters map[TokenType]int
	cache    map[string]string // key: "type:original" -> token
}

// NewTokenSession creates an initialized TokenSession for generating unique DLP-immune tokens.
func NewTokenSession(salt string) *TokenSession {
	return &TokenSession{
		salt:     salt,
		counters: make(map[TokenType]int),
		cache:    make(map[string]string),
	}
}

// GenerateToken deterministically generates or retrieves a unique semantic token for an original value.
func (ts *TokenSession) GenerateToken(tokenType TokenType, original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	cacheKey := fmt.Sprintf("%s:%s", tokenType, trimmed)
	if existing, ok := ts.cache[cacheKey]; ok {
		return existing
	}

	ts.counters[tokenType]++
	idx := ts.counters[tokenType]

	token := fmt.Sprintf("[%s_%04d]", tokenType, idx)
	ts.cache[cacheKey] = token
	return token
}
