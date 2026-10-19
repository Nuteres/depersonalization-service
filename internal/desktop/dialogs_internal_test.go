package desktop

import (
	"testing"
)

func TestParseSavePath(t *testing.T) {
	tests := []struct {
		input        string
		expectedDir  string
		expectedFile string
	}{
		{
			input:        "",
			expectedDir:  "",
			expectedFile: "",
		},
		{
			input:        "report_anonymized.xlsx",
			expectedDir:  "",
			expectedFile: "report_anonymized.xlsx",
		},
		{
			input:        "/home/user/docs/report_anonymized.xlsx",
			expectedDir:  "/home/user/docs",
			expectedFile: "report_anonymized.xlsx",
		},
		{
			input:        `C:\Users\User\Documents\report_anonymized.xlsx`,
			expectedDir:  `C:\Users\User\Documents`,
			expectedFile: "report_anonymized.xlsx",
		},
		{
			input:        "C:/Users/User/Documents/report_anonymized.xlsx",
			expectedDir:  "C:/Users/User/Documents",
			expectedFile: "report_anonymized.xlsx",
		},
		{
			input:        "/file.xlsx",
			expectedDir:  "/",
			expectedFile: "file.xlsx",
		},
		{
			input:        `C:\source.xlsx`,
			expectedDir:  `C:\`,
			expectedFile: "source.xlsx",
		},
		{
			input:        "D:/data.csv",
			expectedDir:  "D:/",
			expectedFile: "data.csv",
		},
	}

	for _, tt := range tests {
		dir, file := parseSavePath(tt.input)
		if dir != tt.expectedDir || file != tt.expectedFile {
			t.Errorf("parseSavePath(%q) = (%q, %q), expected (%q, %q)",
				tt.input, dir, file, tt.expectedDir, tt.expectedFile)
		}
	}
}
