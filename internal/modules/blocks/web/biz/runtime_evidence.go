package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"golang.org/x/net/html"
)

const (
	webEvidenceSchemaVersion  = "workorch.web_evidence.v1"
	webQueryMaxBytes          = 4096
	webURLMaxBytes            = 4096
	webSummaryMaxBytes        = 8192
	webTitleMaxBytes          = 512
	webSnippetMaxBytes        = 4096
	webSourceMaxBytes         = 256
	webPublishedAtMaxBytes    = 64
	webContentTypeMaxBytes    = 256
	webProviderIDMaxBytes     = 128
	webFilterMetadataMaxBytes = 64
	webDomainMaxCount         = 32
	webDomainMaxBytes         = 253
	webEvidenceEnvelopeBytes  = 64 * 1024
)

type webEvidenceEnvelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Kind          string `json:"kind"`
	Payload       any    `json:"payload"`
}

type webSearchEvidencePayload struct {
	ProviderID  string            `json:"providerId"`
	Query       string            `json:"query"`
	Results     []WebSearchResult `json:"results"`
	ResultCount int64             `json:"resultCount"`
	Truncated   bool              `json:"truncated"`
}

type webFetchEvidencePayload struct {
	ProviderID  string `json:"providerId"`
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	Summary     string `json:"summary"`
	ContentType string `json:"contentType,omitempty"`
	Source      string `json:"source,omitempty"`
	Size        int64  `json:"size"`
	Truncated   bool   `json:"truncated"`
}

func validateWebOperationRequest(request RuntimeOperationRequest) error {
	if len(strings.TrimSpace(request.ProviderID)) > webProviderIDMaxBytes ||
		len(strings.TrimSpace(request.Locale)) > webFilterMetadataMaxBytes ||
		len(strings.TrimSpace(request.Recency)) > webFilterMetadataMaxBytes ||
		len(strings.TrimSpace(request.ExtractMode)) > webFilterMetadataMaxBytes {
		return fmt.Errorf("%w: Web provider/filter metadata exceeds owner bounds", common.ErrInvalidArgument)
	}
	for _, values := range [][]string{request.Domains, request.ExcludeDomains} {
		if len(values) > webDomainMaxCount {
			return fmt.Errorf("%w: Web domain filters exceed owner count bound", common.ErrInvalidArgument)
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || len(value) > webDomainMaxBytes {
				return fmt.Errorf("%w: Web domain filter exceeds owner byte bound", common.ErrInvalidArgument)
			}
		}
	}
	switch strings.TrimSpace(request.Kind) {
	case runtimecontract.StepKindWebSearch:
		query := strings.TrimSpace(request.Query)
		if query == "" || len(query) > webQueryMaxBytes || request.MaxResults < 0 || request.MaxResults > webproviderconfig.SystemMaxResults {
			return fmt.Errorf("%w: Web search request exceeds owner bounds", common.ErrInvalidArgument)
		}
	case runtimecontract.StepKindWebFetch:
		rawURL := strings.TrimSpace(request.URL)
		if rawURL == "" || len(rawURL) > webURLMaxBytes || request.MaxBytes < 0 || request.MaxBytes > webproviderconfig.SystemMaxFetchBytes {
			return fmt.Errorf("%w: Web fetch request exceeds owner bounds", common.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: unsupported Web operation kind", common.ErrInvalidArgument)
	}
	return nil
}

func normalizeWebSearchResult(value WebSearchResult) (WebSearchResult, bool) {
	ret := value
	var truncated, clipped bool
	ret.Title, truncated = boundWebText(value.Title, webTitleMaxBytes)
	ret.Snippet, clipped = boundWebText(value.Snippet, webSnippetMaxBytes)
	truncated = truncated || clipped
	ret.Source, clipped = boundWebText(value.Source, webSourceMaxBytes)
	truncated = truncated || clipped
	ret.PublishedAt, clipped = boundWebText(value.PublishedAt, webPublishedAtMaxBytes)
	truncated = truncated || clipped
	ret.URL = strings.TrimSpace(value.URL)
	ret.ContentHash = stableHash(ret.Title, ret.URL, ret.Snippet, ret.Source, ret.PublishedAt)
	return ret, truncated
}

func boundWebText(value string, maxBytes int) (string, bool) {
	value = strings.ToValidUTF8(value, "?")
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return ' '
	}, value)
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	raw := []byte(value[:maxBytes])
	for len(raw) > 0 && !utf8.Valid(raw) {
		raw = raw[:len(raw)-1]
	}
	return strings.TrimSpace(string(raw)), true
}

func buildWebSearchSummary(results []WebSearchResult) (string, bool) {
	var builder strings.Builder
	for index, item := range results {
		if builder.Len() != 0 {
			builder.WriteByte('\n')
		}
		_, _ = fmt.Fprintf(&builder, "%d. %s", index+1, firstWebNonEmpty(item.Title, item.Source, "web result"))
		if item.Snippet != "" {
			builder.WriteString(" — ")
			builder.WriteString(item.Snippet)
		}
		if item.URL != "" {
			builder.WriteString(" [")
			builder.WriteString(item.URL)
			builder.WriteByte(']')
		}
		if builder.Len() > webSummaryMaxBytes {
			break
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("web search returned no results")
	}
	return boundWebText(builder.String(), webSummaryMaxBytes)
}

func summarizeFetchedContent(content string, contentType string) (title string, summary string, truncated bool) {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(contentType, "html") {
		document, err := html.Parse(strings.NewReader(content))
		if err == nil {
			var text strings.Builder
			type pendingNode struct {
				node    *html.Node
				omitted bool
			}
			stack := []pendingNode{{node: document}}
			const maxHTMLNodes = 100_000
			for visited := 0; len(stack) != 0 && visited < maxHTMLNodes; visited++ {
				last := len(stack) - 1
				pending := stack[last]
				stack = stack[:last]
				node, omitted := pending.node, pending.omitted
				if node.Type == html.ElementNode && (strings.EqualFold(node.Data, "script") || strings.EqualFold(node.Data, "style") || strings.EqualFold(node.Data, "noscript")) {
					omitted = true
				}
				if !omitted && node.Type == html.TextNode {
					value := strings.Join(strings.Fields(node.Data), " ")
					if value != "" {
						if text.Len() != 0 {
							text.WriteByte(' ')
						}
						text.WriteString(value)
					}
				}
				if node.Type == html.ElementNode && strings.EqualFold(node.Data, "title") && node.FirstChild != nil {
					title, _ = boundWebText(node.FirstChild.Data, webTitleMaxBytes)
				}
				for child := node.LastChild; child != nil; child = child.PrevSibling {
					stack = append(stack, pendingNode{node: child, omitted: omitted})
				}
			}
			if len(stack) != 0 {
				truncated = true
			}
			var clipped bool
			summary, clipped = boundWebText(text.String(), webSummaryMaxBytes)
			truncated = truncated || clipped
			return title, summary, truncated
		}
	}
	summary, truncated = boundWebText(strings.Join(strings.Fields(content), " "), webSummaryMaxBytes)
	return "", summary, truncated
}

func normalizeFetchedContent(content string) string {
	content = strings.ToValidUTF8(content, "?")
	return strings.Map(func(value rune) rune {
		if value == '\n' || value == '\r' || value == '\t' || !unicode.IsControl(value) {
			return value
		}
		return ' '
	}, content)
}

func persistWebEvidence(internalDirectory string, kind string, payload any) (string, string, int64, error) {
	internalDirectory = strings.TrimSpace(internalDirectory)
	if internalDirectory == "" {
		return "", "", 0, fmt.Errorf("%w: Web evidence store requires workspace internal directory", common.ErrInvalidArgument)
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(webEvidenceEnvelope{SchemaVersion: webEvidenceSchemaVersion, Kind: strings.TrimSpace(kind), Payload: payload}); err != nil {
		return "", "", 0, fmt.Errorf("marshal Web evidence: %w", err)
	}
	raw := encoded.Bytes()
	maxBytes := 2*webproviderconfig.SystemMaxFetchBytes + webEvidenceEnvelopeBytes
	if int64(len(raw)) > maxBytes {
		return "", "", 0, fmt.Errorf("%w: normalized Web evidence exceeds owner byte bound", common.ErrInvalidArgument)
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	directory := filepath.Join(filepath.Clean(internalDirectory), "web", "evidence")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", 0, fmt.Errorf("create Web evidence directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", 0, fmt.Errorf("Web evidence directory is not a regular directory")
	}
	path := filepath.Join(directory, hash+".json")
	if existing, err := readBoundedWebEvidence(path, maxBytes); err == nil {
		if !bytes.Equal(existing, raw) {
			return "", "", 0, fmt.Errorf("%w: Web evidence hash collision", common.ErrConflict)
		}
		return "workorch://web/evidence/" + hash, hash, int64(len(raw)), nil
	} else if !os.IsNotExist(err) {
		return "", "", 0, err
	}
	temporary, err := os.CreateTemp(directory, ".web-evidence-*")
	if err != nil {
		return "", "", 0, fmt.Errorf("create Web evidence temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", "", 0, err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return "", "", 0, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", "", 0, err
	}
	if err := temporary.Close(); err != nil {
		return "", "", 0, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := readBoundedWebEvidence(path, maxBytes); readErr == nil && bytes.Equal(existing, raw) {
			return "workorch://web/evidence/" + hash, hash, int64(len(raw)), nil
		}
		return "", "", 0, fmt.Errorf("commit Web evidence: %w", err)
	}
	return "workorch://web/evidence/" + hash, hash, int64(len(raw)), nil
}

func readBoundedWebEvidence(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("stored Web evidence exceeds owner byte bound")
	}
	return raw, nil
}

func webEvidenceRefs(rawRef string, sourceRefs ...string) []string {
	refs := append([]string{strings.TrimSpace(rawRef)}, sourceRefs...)
	refs = uniqueStrings(refs)
	sort.Strings(refs)
	return refs
}
