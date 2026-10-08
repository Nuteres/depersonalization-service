package formats

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// UTF8BOM is the 3-byte Byte Order Mark for UTF-8 files (\xEF\xBB\xBF).
var UTF8BOM = []byte{0xEF, 0xBB, 0xBF}

// WriteUTF8BOM writes the UTF-8 BOM to w so spreadsheet software (e.g. Microsoft Excel)
// immediately recognizes the file as UTF-8 and correctly renders Cyrillic and Latin characters.
func WriteUTF8BOM(w io.Writer) error {
	_, err := w.Write(UTF8BOM)
	return err
}

// IsValidUTF8Buffer checks if data is valid UTF-8, tolerating partial runes at the tail.
func IsValidUTF8Buffer(data []byte) bool {
	if utf8.Valid(data) {
		return true
	}
	// A peek buffer might be truncated in the middle of a 2..4 byte UTF-8 sequence.
	for i := 1; i <= 3 && i < len(data); i++ {
		if utf8.Valid(data[:len(data)-i]) {
			return true
		}
	}
	return false
}

// DetectAndDecodeCSV inspects r for character encoding
// (UTF-8 with/without BOM, UTF-16LE/BE with BOM, Windows-1251 / CP1251)
// and returns a decoded io.Reader yielding UTF-8 data, along with the detected delimiter.
func DetectAndDecodeCSV(r io.Reader, explicitDelimiter rune) (io.Reader, rune, error) {
	if r == nil {
		return nil, ',', errors.New("reader cannot be nil")
	}

	// If r is an io.ReadSeeker (e.g. *os.File), scan a larger sample to ensure
	// late-appearing non-ASCII characters (e.g. English header with Windows-1251 body) are caught.
	isWindows1251 := false
	if seeker, ok := r.(io.ReadSeeker); ok {
		scanBuf := make([]byte, 65536)
		n, err := seeker.Read(scanBuf)
		if n > 0 {
			sample := scanBuf[:n]
			// Check if it's UTF-16 or UTF-8 with BOM first
			if !bytes.HasPrefix(sample, UTF8BOM) &&
				!bytes.HasPrefix(sample, []byte{0xFF, 0xFE}) &&
				!bytes.HasPrefix(sample, []byte{0xFE, 0xFF}) {
				// If not valid UTF-8, check if there are non-ASCII bytes
				hasNonASCII := false
				for _, b := range sample {
					if b >= 0x80 {
						hasNonASCII = true
						break
					}
				}
				if hasNonASCII && !IsValidUTF8Buffer(sample) {
					isWindows1251 = true
				}
			}
		}
		// Rewind to beginning
		if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, ',', fmt.Errorf("failed to seek reader: %w", seekErr)
			}
		}
	}

	bufReader := bufio.NewReaderSize(r, 8192)

	// Peek leading bytes for BOM or encoding detection
	peekBytes, err := bufReader.Peek(8192)
	if len(peekBytes) == 0 {
		delim := explicitDelimiter
		if delim == 0 {
			delim = ','
		}
		return bufReader, delim, nil
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, ',', fmt.Errorf("failed to peek csv reader: %w", err)
	}

	var decodedReader io.Reader

	if bytes.HasPrefix(peekBytes, UTF8BOM) {
		// UTF-8 with BOM
		_, _ = bufReader.Discard(3)
		decodedReader = bufReader
	} else if bytes.HasPrefix(peekBytes, []byte{0xFF, 0xFE}) {
		// UTF-16LE with BOM
		decodedReader = transform.NewReader(bufReader, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	} else if bytes.HasPrefix(peekBytes, []byte{0xFE, 0xFF}) {
		// UTF-16BE with BOM
		decodedReader = transform.NewReader(bufReader, unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder())
	} else if isWindows1251 || !IsValidUTF8Buffer(peekBytes) {
		// Not valid UTF-8: Decode from Windows-1251 (CP1251) to UTF-8
		decodedReader = transform.NewReader(bufReader, charmap.Windows1251.NewDecoder())
	} else {
		// Valid UTF-8 without BOM (or pure ASCII)
		decodedReader = bufReader
	}

	// Wrap in a buffered reader to auto-detect delimiter from decoded UTF-8 stream
	finalBuf := bufio.NewReader(decodedReader)

	comma := explicitDelimiter
	if comma == 0 {
		comma = ','
		peekDecoded, _ := finalBuf.Peek(2048)
		if len(peekDecoded) > 0 {
			peekStr := strings.TrimPrefix(string(peekDecoded), "\ufeff")
			firstLine := peekStr
			if idx := strings.IndexAny(peekStr, "\r\n"); idx != -1 {
				firstLine = peekStr[:idx]
			}
			semiCount := strings.Count(firstLine, ";")
			tabCount := strings.Count(firstLine, "\t")
			commaCount := strings.Count(firstLine, ",")
			if semiCount > commaCount && semiCount >= tabCount {
				comma = ';'
			} else if tabCount > commaCount && tabCount > semiCount {
				comma = '\t'
			}
		}
	}

	return finalBuf, comma, nil
}

// NewEncodingAwareCSVReader returns a configured *csv.Reader reading from r
// with automatic encoding detection (UTF-8, UTF-8 BOM, UTF-16, Windows-1251)
// and automatic delimiter detection (;, \t, ,).
func NewEncodingAwareCSVReader(r io.Reader, explicitDelimiter rune) (*csv.Reader, rune, error) {
	decodedReader, comma, err := DetectAndDecodeCSV(r, explicitDelimiter)
	if err != nil {
		return nil, ',', err
	}

	csvReader := csv.NewReader(decodedReader)
	csvReader.Comma = comma
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	return csvReader, comma, nil
}
