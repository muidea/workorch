package shell

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellProjectRawStoreProtocol = "shell/project-stage-raw/v1"
const shellProjectRawStoreMaxBytes = 2*runtimecontract.RuntimeValidationOutputChunkMaxBytes + 8192

// 仅保全 owner 的原输出，不是 operation journal 或 Runtime authority。
// scope 仅来自已核验的原 execution 或 owner 内部 reconciliation identity。
type shellProjectRawScope struct {
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	ExecutionKey string
	StageID      string
	Stream       string
}

type shellProjectRawRecord struct {
	ProtocolRef string
	Scope       shellProjectRawScope
	Output      shellProjectRawFact
	SavedSHA256 string
	Chunks      []string `json:",omitempty"`
}

type shellProjectRawStore struct {
	root          *os.Root
	syncDirectory func(*os.Root) error
	linkFile      func(*os.Root, string, string) error
}

func openShellProjectRawStore(internalDirectory string) (*shellProjectRawStore, error) {
	if !filepath.IsAbs(internalDirectory) || filepath.Clean(internalDirectory) != internalDirectory {
		return nil, common.ErrInvalidArgument
	}
	info, err := os.Lstat(internalDirectory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, common.ErrInvalidArgument
	}
	root, err := os.OpenRoot(internalDirectory)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, common.ErrConflict
	}
	return &shellProjectRawStore{root: root, syncDirectory: syncShellProjectRawDirectory, linkFile: func(root *os.Root, old, new string) error { return root.Link(old, new) }}, nil
}

func (s *shellProjectRawStore) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}

func (scope shellProjectRawScope) Validate() error {
	for _, value := range []string{string(scope.WorkspaceID), string(scope.RunID), scope.ExecutionKey, scope.StageID} {
		if value == "" || len(value) > 1024 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return common.ErrInvalidArgument
		}
	}
	if scope.Stream != "stdout" && scope.Stream != "stderr" {
		return common.ErrInvalidArgument
	}
	return nil
}

func (record shellProjectRawRecord) Validate(scope shellProjectRawScope) error {
	if scope.Validate() != nil || record.Scope != scope || record.ProtocolRef != shellProjectRawStoreProtocol {
		return common.ErrConflict
	}
	raw := record.Output
	if shellProjectOutputIdentity(raw).Validate() != nil || raw.SavedBytes > shellProjectRawOutputMaxBytes {
		return common.ErrConflict
	}
	digestBytes, err := hex.DecodeString(record.SavedSHA256)
	if err != nil || len(digestBytes) != sha256.Size || hex.EncodeToString(digestBytes) != record.SavedSHA256 || raw.Complete && raw.SHA256 != record.SavedSHA256 {
		return common.ErrConflict
	}
	if len(record.Chunks) != 0 {
		if raw.Raw != nil || raw.SavedBytes <= runtimecontract.RuntimeValidationOutputChunkMaxBytes || len(record.Chunks) != int((raw.SavedBytes+runtimecontract.RuntimeValidationOutputChunkMaxBytes-1)/runtimecontract.RuntimeValidationOutputChunkMaxBytes) {
			return common.ErrConflict
		}
		for _, chunk := range record.Chunks {
			digest, err := hex.DecodeString(chunk)
			if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != chunk {
				return common.ErrConflict
			}
		}
	} else {
		if raw.SavedBytes != int64(len(raw.Raw)) || len(raw.Raw) > runtimecontract.RuntimeValidationOutputChunkMaxBytes {
			return common.ErrConflict
		}
		digest := sha256.Sum256(raw.Raw)
		if record.SavedSHA256 != hex.EncodeToString(digest[:]) {
			return common.ErrConflict
		}
	}

	return nil
}

func shellProjectRawScopeDirectory(scope shellProjectRawScope) string {
	body, _ := json.Marshal(scope)
	digest := sha256.Sum256(body)
	return "shell/project-stage-raw/" + hex.EncodeToString(digest[:])
}

func (s *shellProjectRawStore) directory(scope shellProjectRawScope, create bool) (*os.Root, error) {
	if s == nil || s.root == nil || scope.Validate() != nil {
		return nil, common.ErrInvalidArgument
	}
	root, err := s.root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	for name := range strings.SplitSeq(shellProjectRawScopeDirectory(scope), "/") {
		if create {
			if err := root.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				_ = root.Close()
				return nil, err
			}
		}
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			_ = root.Close()
			if err != nil {
				return nil, err
			}
			return nil, common.ErrConflict
		}
		if create {
			// 重试时也同步父目录，不能把上次仅可见的 mkdir 当持久回执。
			if err := s.syncDirectory(root); err != nil {
				_ = root.Close()
				return nil, err
			}
		}
		next, err := root.OpenRoot(name)
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) || opened.Mode().Perm()&0o077 != 0 {
			_ = next.Close()
			return nil, common.ErrConflict
		}
		root = next
	}
	return root, nil
}

func (s *shellProjectRawStore) Store(ctx context.Context, scope shellProjectRawScope, raw shellProjectRawFact) (string, error) {
	if ctx == nil || scope.Validate() != nil {
		return "", common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if shellProjectOutputIdentity(raw).Validate() != nil || raw.SavedBytes != int64(len(raw.Raw)) || len(raw.Raw) > shellProjectRawOutputMaxBytes {
		return "", common.ErrConflict
	}
	digest := sha256.Sum256(raw.Raw)
	record := shellProjectRawRecord{ProtocolRef: shellProjectRawStoreProtocol, Scope: scope, Output: raw, SavedSHA256: hex.EncodeToString(digest[:])}
	if raw.Complete && raw.SHA256 != record.SavedSHA256 {
		return "", common.ErrConflict
	}
	if len(raw.Raw) > runtimecontract.RuntimeValidationOutputChunkMaxBytes {
		record.Output.Raw = nil
		for offset := 0; offset < len(raw.Raw); offset += runtimecontract.RuntimeValidationOutputChunkMaxBytes {
			chunk := raw.Raw[offset:min(len(raw.Raw), offset+runtimecontract.RuntimeValidationOutputChunkMaxBytes)]
			hash := sha256.Sum256(chunk)
			record.Chunks = append(record.Chunks, hex.EncodeToString(hash[:]))
		}
	}
	if err := record.Validate(scope); err != nil {
		return "", err
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) > shellProjectRawStoreMaxBytes {
		return "", common.ErrInvalidArgument
	}
	identity := sha256.Sum256(body)
	ref := hex.EncodeToString(identity[:])
	root, err := s.directory(scope, true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	for index, hash := range record.Chunks {
		offset := index * runtimecontract.RuntimeValidationOutputChunkMaxBytes
		chunk := raw.Raw[offset:min(len(raw.Raw), offset+runtimecontract.RuntimeValidationOutputChunkMaxBytes)]
		if err := s.publish(ctx, root, hash+".bin", chunk); err != nil {
			return "", err
		}
	}
	if err := s.publish(ctx, root, ref+".json", body); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *shellProjectRawStore) publish(ctx context.Context, root *os.Root, name string, body []byte) error {
	if existing, err := readShellProjectRawFile(root, name, shellProjectRawStoreMaxBytes); err == nil {
		if !bytes.Equal(existing, body) {
			return common.ErrConflict
		}
		if err := s.syncDirectory(root); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := ".raw-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temporary) }()
	n, writeErr := file.Write(body)
	if writeErr == nil && n != len(body) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 不覆盖已有原文。并发发布只能精确复用同 bytes，绝不换掉旧内容。
	if err := s.linkFile(root, temporary, name); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, err := readShellProjectRawFile(root, name, shellProjectRawStoreMaxBytes)
		if err != nil || !bytes.Equal(existing, body) {
			return common.ErrConflict
		}
	}
	if err := root.Remove(temporary); err != nil {
		return err
	}
	if err := s.syncDirectory(root); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *shellProjectRawStore) readMetadata(ctx context.Context, scope shellProjectRawScope, ref string) (shellProjectRawRecord, *os.Root, error) {
	if ctx == nil || scope.Validate() != nil || len(ref) != sha256.Size*2 || strings.ToLower(ref) != ref {
		return shellProjectRawRecord{}, nil, common.ErrInvalidArgument
	}
	if _, err := hex.DecodeString(ref); err != nil {
		return shellProjectRawRecord{}, nil, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return shellProjectRawRecord{}, nil, err
	}
	root, err := s.directory(scope, false)
	if err != nil {
		return shellProjectRawRecord{}, nil, err
	}
	fail := func(err error) (shellProjectRawRecord, *os.Root, error) {
		root.Close()
		return shellProjectRawRecord{}, nil, err
	}
	body, err := readShellProjectRawRecord(root, ref+".json")
	if err != nil {
		return fail(err)
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != ref {
		return fail(common.ErrConflict)
	}
	var record shellProjectRawRecord
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return fail(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fail(common.ErrConflict)
	}
	if err := record.Validate(scope); err != nil {
		return fail(err)
	}
	return record, root, nil
}

func readShellProjectChunk(root *os.Root, record shellProjectRawRecord, index int) ([]byte, error) {
	body, err := readShellProjectRawFile(root, record.Chunks[index]+".bin", runtimecontract.RuntimeValidationOutputChunkMaxBytes)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	want := min(int64(runtimecontract.RuntimeValidationOutputChunkMaxBytes), record.Output.SavedBytes-int64(index*runtimecontract.RuntimeValidationOutputChunkMaxBytes))
	if int64(len(body)) != want || hex.EncodeToString(digest[:]) != record.Chunks[index] {
		return nil, common.ErrConflict
	}
	return body, nil
}

// ReadChunk never reads or decodes the rest of a large stream. The immutable
// metadata binds the source and chunk hashes; only selected chunk bytes are read.
func (s *shellProjectRawStore) ReadChunk(ctx context.Context, scope shellProjectRawScope, ref string, offset int64) (shellProjectRawRecord, []byte, error) {
	record, root, err := s.readMetadata(ctx, scope, ref)
	if err != nil {
		return shellProjectRawRecord{}, nil, err
	}
	defer root.Close()
	if offset < 0 || offset > record.Output.SavedBytes {
		return shellProjectRawRecord{}, nil, common.ErrInvalidArgument
	}
	end := min(record.Output.SavedBytes, offset+runtimecontract.RuntimeValidationOutputChunkMaxBytes)
	var raw []byte
	if len(record.Chunks) == 0 {
		raw = append([]byte(nil), record.Output.Raw[offset:end]...)
	} else {
		for pos := offset; pos < end; {
			index := int(pos / runtimecontract.RuntimeValidationOutputChunkMaxBytes)
			chunk, err := readShellProjectChunk(root, record, index)
			if err != nil {
				return shellProjectRawRecord{}, nil, err
			}
			start := pos % runtimecontract.RuntimeValidationOutputChunkMaxBytes
			count := min(int64(len(chunk))-start, end-pos)
			raw = append(raw, chunk[start:start+count]...)
			pos += count
		}
	}
	return record, raw, ctx.Err()
}

func (s *shellProjectRawStore) Read(ctx context.Context, scope shellProjectRawScope, ref string) (shellProjectRawFact, error) {
	record, root, err := s.readMetadata(ctx, scope, ref)
	if err != nil {
		return shellProjectRawFact{}, err
	}
	defer root.Close()
	raw := record.Output
	if len(record.Chunks) != 0 {
		for index := range record.Chunks {
			chunk, err := readShellProjectChunk(root, record, index)
			if err != nil {
				return shellProjectRawFact{}, err
			}
			raw.Raw = append(raw.Raw, chunk...)
		}
	}
	digest := sha256.Sum256(raw.Raw)
	if int64(len(raw.Raw)) != raw.SavedBytes || hex.EncodeToString(digest[:]) != record.SavedSHA256 {
		return shellProjectRawFact{}, common.ErrConflict
	}
	return raw, ctx.Err()
}

func readShellProjectRawRecord(root *os.Root, name string) ([]byte, error) {
	return readShellProjectRawFile(root, name, shellProjectRawStoreMaxBytes)
}
func readShellProjectRawFile(root *os.Root, name string, maxBytes int) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > int64(maxBytes) {
		return nil, common.ErrConflict
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 || opened.Size() > int64(maxBytes) {
		return nil, common.ErrConflict
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("Shell 原文记录超过明确存储边界")
	}
	return body, nil
}

func syncShellProjectRawDirectory(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
