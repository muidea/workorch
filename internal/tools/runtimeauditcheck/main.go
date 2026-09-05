package main

import (
	"encoding/json"
	"fmt"
	"os"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchruntimeaudit"
)

type request struct {
	Audit           *projectioncontract.RuntimeAudit `json:"audit"`
	RequireComplete bool                             `json:"requireComplete"`
}

type response struct {
	Errors []string `json:"errors"`
}

func main() {
	var input request
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "decode runtime audit request: %v\n", err)
		os.Exit(2)
	}
	result := response{Errors: workorchruntimeaudit.CoverageBindingChainErrors(input.Audit, input.RequireComplete)}
	if result.Errors == nil {
		result.Errors = []string{}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode runtime audit result: %v\n", err)
		os.Exit(2)
	}
}
