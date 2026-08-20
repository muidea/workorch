package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

const fileStreamBufferBytes = 32 * 1024

type boundedFileScan struct {
	Content     []byte
	ContentHash string
	Size        int64
	LineCount   int64
	Truncated   bool
	Binary      bool
}

func scanBoundedFile(ctx context.Context, path string, startLine, endLine int64, maxBytes int) (boundedFileScan, error) {
	if path == "" || maxBytes <= 0 {
		return boundedFileScan{}, fmt.Errorf("invalid bounded file scan")
	}
	file, err := os.Open(path)
	if err != nil {
		return boundedFileScan{}, err
	}
	defer file.Close()

	ranged := startLine > 0 || endLine > 0
	if startLine <= 0 {
		startLine = 1
	}
	currentLine := int64(1)
	hash := sha256.New()
	content := make([]byte, 0, min(maxBytes, fileStreamBufferBytes))
	buffer := make([]byte, fileStreamBufferBytes)
	validator := streamingUTF8Validator{}
	var size, lineBreaks int64
	var lastByte byte
	hasNUL := false
	truncated := false

	appendBounded := func(value []byte) {
		remaining := maxBytes - len(content)
		if remaining > 0 {
			count := min(remaining, len(value))
			content = append(content, value[:count]...)
			value = value[count:]
		}
		if len(value) > 0 {
			truncated = true
		}
	}
	consumeSelected := func(value []byte) {
		if !ranged {
			appendBounded(value)
			return
		}
		for len(value) > 0 {
			newline := bytes.IndexByte(value, '\n')
			end := len(value)
			if newline >= 0 {
				end = newline + 1
			}
			if currentLine >= startLine && (endLine <= 0 || currentLine <= endLine) {
				appendBounded(value[:end])
			}
			value = value[end:]
			if newline >= 0 {
				currentLine++
			}
		}
	}

	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return boundedFileScan{}, err
			}
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			_, _ = hash.Write(chunk)
			size += int64(count)
			lineBreaks += int64(bytes.Count(chunk, []byte{'\n'}))
			lastByte = chunk[len(chunk)-1]
			hasNUL = hasNUL || bytes.IndexByte(chunk, 0) >= 0
			validator.Consume(chunk)
			consumeSelected(chunk)
		}
		if readErr != nil {
			if readErr != io.EOF {
				return boundedFileScan{}, readErr
			}
			break
		}
	}
	lineCount := lineBreaks
	if size > 0 && lastByte != '\n' {
		lineCount++
	}
	return boundedFileScan{
		Content: content, ContentHash: hex.EncodeToString(hash.Sum(nil)), Size: size, LineCount: lineCount,
		Truncated: truncated, Binary: hasNUL || !validator.Valid(),
	}, nil
}

func streamFileHashAndBinary(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, fileStreamBufferBytes)
	validator := streamingUTF8Validator{}
	hasNUL := false
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			_, _ = hash.Write(chunk)
			hasNUL = hasNUL || bytes.IndexByte(chunk, 0) >= 0
			validator.Consume(chunk)
		}
		if readErr != nil {
			if readErr != io.EOF {
				return "", false, readErr
			}
			break
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), hasNUL || !validator.Valid(), nil
}

// streamingUTF8Validator keeps at most one split rune between fixed-size reads.
// It preserves the former whole-file UTF-8 classification without retaining the
// whole file in memory.
type streamingUTF8Validator struct {
	tail    [utf8.UTFMax]byte
	tailLen int
	invalid bool
}

func (v *streamingUTF8Validator) Consume(value []byte) {
	if v == nil || v.invalid || len(value) == 0 {
		return
	}
	if v.tailLen > 0 {
		consumed := 0
		for consumed < len(value) && !utf8.FullRune(v.tail[:v.tailLen]) {
			v.tail[v.tailLen] = value[consumed]
			v.tailLen++
			consumed++
		}
		if utf8.FullRune(v.tail[:v.tailLen]) {
			runeValue, size := utf8.DecodeRune(v.tail[:v.tailLen])
			if runeValue == utf8.RuneError && size == 1 {
				v.invalid = true
				return
			}
			v.tailLen = 0
		}
		value = value[consumed:]
	}
	if len(value) == 0 {
		return
	}
	validEnd := len(value)
	start := len(value) - 1
	lowerBound := len(value) - utf8.UTFMax
	if lowerBound < 0 {
		lowerBound = 0
	}
	for start >= lowerBound && !utf8.RuneStart(value[start]) {
		start--
	}
	if start >= lowerBound && !utf8.FullRune(value[start:]) {
		validEnd = start
	}
	if !utf8.Valid(value[:validEnd]) {
		v.invalid = true
		return
	}
	v.tailLen = copy(v.tail[:], value[validEnd:])
}

func (v streamingUTF8Validator) Valid() bool {
	return !v.invalid && v.tailLen == 0
}
