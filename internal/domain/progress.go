package domain

// ProgressReporter reports progress of long-running operations.
// processed: number of rows/units processed so far.
// total: total number of rows/units (-1 if unknown).
// percent: completion percentage (0-100).
type ProgressReporter func(processed, total, percent int)
