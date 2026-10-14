package domain

// ProcessingMode defines the processing strategy for sensitive personal data.
type ProcessingMode string

const (
	ModeReplace ProcessingMode = "replace"
	ModeDelete  ProcessingMode = "delete"
)

// AnonymizationStrategy defines the anonymization method: semantic tokens (DLP-safe), synthetic values, or redaction.
type AnonymizationStrategy string

const (
	StrategySemanticTokens AnonymizationStrategy = "tokens"
	StrategySynthetic      AnonymizationStrategy = "synthetic"
	StrategyDelete         AnonymizationStrategy = "delete"
)

// PDFRedactionBox specifies the exact page and coordinates (PDF points 72 DPI) for a redaction blackout plate.
type PDFRedactionBox struct {
	Page   int     `json:"page"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ProcessOptions specifies options for processing and anonymizing a file.
type ProcessOptions struct {
	InputPath         string                `json:"inputPath"`
	OutputPath        string                `json:"outputPath"`
	Mode              ProcessingMode        `json:"mode"`
	Strategy          AnonymizationStrategy `json:"strategy,omitempty"`
	Salt              string                `json:"salt"`
	DisabledCols      []int                 `json:"disabledCols"`
	DisabledBySheet   map[string][]int      `json:"disabledBySheet"`
	CreateKey         bool                  `json:"createKey"`
	KeyPassword       string                `json:"keyPassword"`
	KeyPath           string                `json:"keyPath"`
	ShuffleRows       bool                  `json:"shuffleRows"`
	PDFRedactionBoxes []PDFRedactionBox     `json:"pdfRedactionBoxes,omitempty"`
}

// RestoreOptions specifies options for restoring a previously anonymized file using a .depmap key.
type RestoreOptions struct {
	InputPath  string `json:"inputPath"`
	KeyPath    string `json:"keyPath"`
	Password   string `json:"password"`
	OutputPath string `json:"outputPath"`
}
