package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
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
