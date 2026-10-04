package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"strconv"
	"unicode/utf8"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// 位于 wire limit reader 内侧，包含其实际读取但未向 parser 返回的探测字节。
// 超出保存界限时不留前缀；hash 和长度仍只覆盖实际观察范围。
type providerDocumentObservationReader struct {
	reader  io.Reader
	hash    hash.Hash
	bytes   uint64
	content []byte
	eof     bool
	read    bool
}

func newProviderDocumentObservationReader(reader io.Reader) *providerDocumentObservationReader {
	return &providerDocumentObservationReader{reader: reader, hash: sha256.New(), content: []byte{}}
}

func (r *providerDocumentObservationReader) Read(buffer []byte) (int, error) {
	r.read = true
	n, err := r.reader.Read(buffer)
	if n > 0 {
		r.bytes += uint64(n)
		_, _ = r.hash.Write(buffer[:n])
		if r.bytes <= llmmodels.ProviderDocumentObservationLimit {
			r.content = append(r.content, buffer[:n]...)
		} else {
			r.content = nil
		}
	}
	if err == io.EOF {
		r.eof = true
	}
	return n, err
}

func (r *providerDocumentObservationReader) observation() *llmmodels.ProviderObservedContent {
	if !r.read {
		return &llmmodels.ProviderObservedContent{Reason: "response_body_not_read"}
	}
	o := llmmodels.ProviderObservedContent{Observed: true, ObservedBytes: r.bytes, SHA256: hex.EncodeToString(r.hash.Sum(nil)), Complete: r.eof, Saved: r.content != nil}
	if !r.eof {
		o.Reason = "response_not_fully_read"
	}
	if o.Saved {
		o.Content = append([]byte{}, r.content...)
	} else {
		o.Reason = "observation_storage_limit"
	}
	return &o
}

// encoding/json 会将非法 UTF-8 和孤立 surrogate 替换为 U+FFFD；文档观测禁止这种有损解码。
// JSON 结构仍由原协议 decoder 校验，这里仅反证字符串编码。
func providerJSONTextLossless(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	inside := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		value, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func providerDocumentObservation(content string, complete bool) *llmmodels.ProviderObservedContent {
	reason := ""
	if !complete {
		reason = "document_incomplete"
	}
	observation := llmmodels.NewProviderObservedContent([]byte(content), complete, reason)
	return &observation
}
