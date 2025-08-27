package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync/atomic"
)

// ----- JSON-RPC 2.0 -----
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      any              `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *JSONRPCErrorObj `json:"error,omitempty"`
}

type JSONRPCErrorObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func writeResp(w io.Writer, id any, result any, errObj *JSONRPCErrorObj) error {
	resp := JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result, Error: errObj}
	enc := json.NewEncoder(w)
	return enc.Encode(resp)
}

// ----- MCP minimal: structures -----

// initialize → result
type InitializeResult struct {
	ProtocolVersion string        `json:"protocolVersion"` // e.g. "2024-11-05" (contoh)
	Capabilities    Capabilities  `json:"capabilities"`
	ServerInfo      MCPServerInfo `json:"serverInfo"`
}
type Capabilities struct {
	Tools bool `json:"tools"`
	// Tambahkan bila perlu: "resources", "prompts", dll.
}
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// tools/list → result
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	// optional: outputSchema
}

// tools/call → params & result
type ToolsCallParams struct {
	Name string         `json:"name"`
	Args map[string]any `json:"arguments"`
	// optional: "timeout", "idempotent", etc.
}
type ToolsCallResult struct {
	Content any `json:"content"`
}

// Util baca/loop stdio
type StdioRPC struct {
	r *bufio.Reader
	w io.Writer
}

func NewStdioRPC() *StdioRPC {
	return &StdioRPC{
		r: bufio.NewReader(os.Stdin),
		w: os.Stdout,
	}
}

func (s *StdioRPC) Read() (*JSONRPCRequest, error) {
	dec := json.NewDecoder(s.r)
	var req JSONRPCRequest
	if err := dec.Decode(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *StdioRPC) Reply(id any, result any) error {
	return writeResp(s.w, id, result, nil)
}
func (s *StdioRPC) ReplyError(id any, code int, msg string, data any) error {
	return writeResp(s.w, id, nil, &JSONRPCErrorObj{Code: code, Message: msg, Data: data})
}

// Helper ID bila perlu (tidak dipakai di contoh ini)
var rid int64

func nextID() int64 { return atomic.AddInt64(&rid, 1) }
