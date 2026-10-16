package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBusinessJSONResponsesDisablePersistentCache(t *testing.T) {
	for name, write := range map[string]func(http.ResponseWriter){
		"artifact": func(w http.ResponseWriter) {
			writeArtifactContent(w, content.ArtifactContent{Content: []byte("原始产物")})
		},
		"runtime": func(w http.ResponseWriter) {
			writeRuntimeRawContent(w, content.RuntimeRawContent{Content: []byte("原始诊断")})
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			write(response)
			if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("raw response permits persistent cache: status=%d headers=%v", response.Code, response.Header())
			}
		})
	}
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusBadRequest, http.StatusNotFound, http.StatusConflict} {
		response := httptest.NewRecorder()
		writeJSON(response, status, struct {
			Value string `json:"value"`
		}{Value: "原始业务内容"})
		if response.Code != status || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("business response permits persistent cache: status=%d headers=%v", response.Code, response.Header())
		}
	}
}

func TestRuntimeRawOutputHTTPPreservesBinaryBytesAndReportsIncompleteStorage(t *testing.T) {
	raw := []byte{0, 255, '\r', '\n'}
	response := httptest.NewRecorder()
	writeRuntimeRawContent(response, content.RuntimeRawContent{
		Ref: "owner-read-ref", FileName: "stdout.bin", Content: raw, ContentSHA256: "saved-hash",
		OutputIdentity: &runtimecontract.RuntimeValidationOutputIdentity{Bytes: 8, SavedBytes: 4, SHA256: "original-hash", Complete: false, OmittedReason: "output_capacity_exceeded", Storage: &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "storage_unavailable"}},
	})
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), raw) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("raw output changed or cached: status=%d body=%v headers=%v", response.Code, response.Body.Bytes(), response.Header())
	}
	for key, want := range map[string]string{
		"X-Workorch-Content-SHA256": "saved-hash", "X-Workorch-Output-SHA256": "original-hash",
		"X-Workorch-Output-Bytes": "8", "X-Workorch-Output-Saved-Bytes": "4",
		"X-Workorch-Output-Complete": "false", "X-Workorch-Output-Omitted-Reason": "output_capacity_exceeded",
		"X-Workorch-Output-Storage-Omitted-Reason": "storage_unavailable",
	} {
		if got := response.Header().Get(key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
}
