package domain

// ProcessOptions specifies options for processing and anonymizing a file.
type ProcessOptions struct {
	InputPath       string           `json:"inputPath"`
	OutputPath      string           `json:"outputPath"`
	Salt            string           `json:"salt"`
	DisabledCols    []int            `json:"disabledCols"`
	DisabledBySheet map[string][]int `json:"disabledBySheet"`
	CreateKey       bool             `json:"createKey"`
	KeyPassword     string           `json:"keyPassword"`
	KeyPath         string           `json:"keyPath"`
}

// RestoreOptions specifies options for restoring a previously anonymized file using a .depmap key.
type RestoreOptions struct {
	InputPath  string `json:"inputPath"`
	KeyPath    string `json:"keyPath"`
	Password   string `json:"password"`
	OutputPath string `json:"outputPath"`
}
