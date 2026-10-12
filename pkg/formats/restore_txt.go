package formats

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"dep_service/pkg/depmap"
)

// RestoreTXT restores de-identified plain text files by looking up tokens in dm.Mapping.
func RestoreTXT(ctx context.Context, r io.Reader, w io.Writer, dm *depmap.DepMap) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dm == nil {
		return 0, errors.New("depmap cannot be nil")
	}

	scanner := bufio.NewScanner(r)
	writer := bufio.NewWriter(w)
	defer writer.Flush()

	reTokens := regexp.MustCompile(`\[[А-Яа-яA-Za-z0-9_]+\]`)
	restoredCount := 0
	firstLine := true

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		line := scanner.Text()
		matches := reTokens.FindAllString(line, -1)
		newLine := line

		if len(matches) > 0 {
			for _, m := range matches {
				if orig, ok := dm.Get(m); ok {
					newLine = strings.ReplaceAll(newLine, m, orig)
					restoredCount++
				}
			}
		} else if orig, ok := dm.Get(line); ok {
			newLine = orig
			restoredCount++
		}

		if !firstLine {
			if _, err := writer.WriteString("\n"); err != nil {
				return restoredCount, err
			}
		}
		if _, err := writer.WriteString(newLine); err != nil {
			return restoredCount, err
		}
		firstLine = false
	}

	if err := scanner.Err(); err != nil {
		return restoredCount, fmt.Errorf("error reading txt stream: %w", err)
	}

	return restoredCount, nil
}
