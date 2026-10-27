package biz

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func preparationHTTPWorkflowRequest(t *testing.T, baseURL, method, path string, intent management.PreparationCheckRequest) management.PreparationOperationView {
	t.Helper()
	payload, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body management.PreparationOperationView
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("HTTP preparation: status=%d err=%v %+v", response.StatusCode, err, body)
	}
	if method != http.MethodGet && body.RequestID != intent.RequestID {
		t.Fatal("POST lost original identity")
	}
	return body
}
