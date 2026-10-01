package domain

// ProcessResult holds the outcome of a batch anonymization run.
type ProcessResult struct {
	Success       bool   `json:"success"`
	InputPath     string `json:"inputPath"`
	OutputPath    string `json:"outputPath"`
	KeyPath       string `json:"keyPath,omitempty"`
	RowsCount     int    `json:"rowsCount"`
	EntitiesCount int    `json:"entitiesCount"`
	DurationMs    int64  `json:"durationMs"`
	Message       string `json:"message"`
	Error         string `json:"error,omitempty"`
}

// KeyFileInfo holds metadata and validation status of a .depmap key container.
type KeyFileInfo struct {
	Path             string `json:"path"`
	OriginalFileName string `json:"originalFileName"`
	CreatedAt        string `json:"createdAt"`
	RecordsCount     int    `json:"recordsCount"`
	Valid            bool   `json:"valid"`
	Error            string `json:"error,omitempty"`
}

// RestoreResult holds the outcome of a reverse depersonalization run.
type RestoreResult struct {
	Success       bool   `json:"success"`
	InputPath     string `json:"inputPath"`
	KeyPath       string `json:"keyPath"`
	OutputPath    string `json:"outputPath"`
	RestoredCells int    `json:"restoredCells"`
	DurationMs    int64  `json:"durationMs"`
	Error         string `json:"error,omitempty"`
}
