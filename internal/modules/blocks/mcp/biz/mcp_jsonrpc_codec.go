package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func mcpArgumentsJSON(values []runtimecontract.OperationParameter) (map[string]json.RawMessage, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ret := make(map[string]json.RawMessage, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		var raw []byte
		var err error
		switch {
		case value.String != nil:
			raw, err = json.Marshal(*value.String)
		case value.Integer != nil:
			raw, err = json.Marshal(*value.Integer)
		case value.Boolean != nil:
			raw, err = json.Marshal(*value.Boolean)
		case value.StringList != nil:
			raw, err = json.Marshal(value.StringList)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		ret[name] = raw
	}
	return ret, nil
}

func writeJSONRPCLine(w io.Writer, payload jsonRPCRequest) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = w.Write(raw)
	return err
}

func readJSONRPCResponseWithContext(ctx context.Context, reader *bufio.Reader, expectedID int, session *stdioSession) (jsonRPCResponse, []MCPStreamEvent, error) {
	type responseResult struct {
		response jsonRPCResponse
		events   []MCPStreamEvent
		err      error
	}
	done := make(chan responseResult, 1)
	go func() {
		response, events, err := readJSONRPCResponse(reader, expectedID)
		done <- responseResult{response: response, events: events, err: err}
	}()
	select {
	case result := <-done:
		return result.response, result.events, result.err
	case <-ctx.Done():
		if session != nil {
			session.Close()
		}
		return jsonRPCResponse{}, nil, ctx.Err()
	}
}

func readJSONRPCResponse(reader *bufio.Reader, expectedID int) (jsonRPCResponse, []MCPStreamEvent, error) {
	events := []MCPStreamEvent{}
	totalBytes := 0
	for {
		line, err := readBoundedJSONRPCLine(reader, maxMCPResponseBytes)
		if err != nil {
			return jsonRPCResponse{}, events, err
		}
		totalBytes += len(line)
		if totalBytes > maxMCPResponseBytes {
			return jsonRPCResponse{}, events, fmt.Errorf("%w: mcp stdio response exceeds owner output limit", common.ErrInvalidArgument)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var response jsonRPCResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return jsonRPCResponse{}, events, fmt.Errorf("%w: decode mcp stdio json-rpc response: %v", common.ErrInvalidArgument, err)
		}
		if jsonRPCIDMatches(response.ID, expectedID) {
			if response.Error != nil {
				return response, events, fmt.Errorf("mcp json-rpc returned an error response")
			}
			return response, events, nil
		}
		if response.Method != "" {
			if len(events) == maxMCPStreamEvents {
				return jsonRPCResponse{}, events, fmt.Errorf("%w: mcp stdio notification count exceeds owner limit", common.ErrInvalidArgument)
			}
			events = append(events, MCPStreamEvent{Method: response.Method, Params: append(json.RawMessage(nil), response.Params...)})
		}
	}
}

func readBoundedJSONRPCLine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, common.ErrInvalidArgument
	}
	line := make([]byte, 0, min(reader.Size(), maxBytes))
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxBytes {
			return nil, fmt.Errorf("%w: mcp stdio response line exceeds owner output limit", common.ErrInvalidArgument)
		}
		line = append(line, fragment...)
		switch err {
		case nil:
			return line, nil
		case bufio.ErrBufferFull:
			continue
		default:
			return nil, err
		}
	}
}

func jsonRPCIDMatches(raw json.RawMessage, expected int) bool {
	if len(raw) == 0 {
		return false
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number == expected
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text == fmt.Sprintf("%d", expected)
	}
	var floating float64
	if err := json.Unmarshal(raw, &floating); err == nil {
		return int(floating) == expected
	}
	return false
}
