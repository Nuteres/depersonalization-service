package depmap_test

import (
	"errors"
	"testing"

	"dep_service/pkg/depmap"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     error
		shouldPass  bool
	}{
		{
			name:       "empty password",
			password:   "",
			wantErr:    depmap.ErrEmptyPassword,
			shouldPass: false,
		},
		{
			name:       "whitespace only",
			password:   "   ",
			wantErr:    depmap.ErrEmptyPassword,
			shouldPass: false,
		},
		{
			name:       "too short (< 12 chars)",
			password:   "Pass1!",
			wantErr:    depmap.ErrPasswordTooShort,
			shouldPass: false,
		},
		{
			name:       "missing uppercase",
			password:   "pass1234567!#",
			wantErr:    depmap.ErrPasswordNoUppercase,
			shouldPass: false,
		},
		{
			name:       "missing lowercase",
			password:   "PASS1234567!#",
			wantErr:    depmap.ErrPasswordNoLowercase,
			shouldPass: false,
		},
		{
			name:       "missing digits",
			password:   "PasswordSecret!#",
			wantErr:    depmap.ErrPasswordNoDigit,
			shouldPass: false,
		},
		{
			name:       "missing special character",
			password:   "PasswordSecret123",
			wantErr:    depmap.ErrPasswordNoSpecial,
			shouldPass: false,
		},
		{
			name:       "valid standard strong password",
			password:   "Correct#Pass2026!",
			wantErr:    nil,
			shouldPass: true,
		},
		{
			name:       "invalid with cyrillic letters",
			password:   "СекретныйПароль2026!",
			wantErr:    depmap.ErrPasswordOnlyLatin,
			shouldPass: false,
		},
		{
			name:       "invalid mixed latin and cyrillic homoglyph",
			password:   "Pаssword12345!", // contains cyrillic 'а'
			wantErr:    depmap.ErrPasswordOnlyLatin,
			shouldPass: false,
		},
		{
			name:       "invalid with spaces",
			password:   "Correct Pass 2026!",
			wantErr:    depmap.ErrPasswordOnlyLatin,
			shouldPass: false,
		},
		{
			name:       "valid 12 char minimum",
			password:   "Abc12345678!",
			wantErr:    nil,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := depmap.ValidatePassword(tt.password)
			if tt.shouldPass {
				if err != nil {
					t.Fatalf("expected password to pass, but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error, but got nil")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestGenerateRandomPassword_SatisfiesPolicy(t *testing.T) {
	// Generate 100 passwords and verify all 100 satisfy ValidatePassword
	for i := 0; i < 100; i++ {
		pwd, err := depmap.GenerateRandomPassword(16)
		if err != nil {
			t.Fatalf("failed to generate random password: %v", err)
		}
		if len(pwd) != 16 {
			t.Fatalf("expected length 16, got %d", len(pwd))
		}
		if err := depmap.ValidatePassword(pwd); err != nil {
			t.Fatalf("generated password %q failed validation: %v", pwd, err)
		}
	}
}

func TestEvaluatePasswordStrength(t *testing.T) {
	tests := []struct {
		password      string
		expectedScore int
	}{
		{"", 0},
		{"short", 1},
		{"СекретныйПароль2026!", 1},
		{"LowerAndUpperWord", 2},
		{"LowerUpperDigits123", 3},
		{"ValidStrongPass2026!", 4},
	}

	for _, tt := range tests {
		score, details := depmap.EvaluatePasswordStrength(tt.password)
		if score != tt.expectedScore {
			t.Errorf("for password %q expected score %d, got %d (details: %+v)", tt.password, tt.expectedScore, score, details)
		}
	}
}
