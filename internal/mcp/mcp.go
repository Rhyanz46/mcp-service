package mcp

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strconv"
    "strings"
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
    ProtocolVersion string       `json:"protocolVersion"`
    Capabilities    Capabilities `json:"capabilities"`
    ServerInfo      MCPServerInfo `json:"serverInfo"`
}

type Capabilities struct {
    // Per MCP spec, capabilities are objects; an empty object means supported.
    Tools map[string]any `json:"tools"`
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
}

// tools/call → params & result
type ToolsCallParams struct {
    Name string         `json:"name"`
    Args map[string]any `json:"arguments"`
}

// ContentItem represents a single content part in MCP responses
type ContentItem struct {
    Type string `json:"type"`
    // Text content
    Text string `json:"text,omitempty"`
    // JSON content (structured)
    JSON any `json:"json,omitempty"`
}

type ToolsCallResult struct {
    Content []ContentItem `json:"content"`
}

// Util baca/loop stdio
type StdioRPC struct {
    r *bufio.Reader
    w io.Writer
    headerMode bool
}

func NewStdioRPC() *StdioRPC {
    return &StdioRPC{
        r: bufio.NewReader(os.Stdin),
        w: os.Stdout,
    }
}

func (s *StdioRPC) Read() (*JSONRPCRequest, error) {
    // Detect framing
    b, err := s.r.Peek(1)
    if err != nil {
        return nil, err
    }
    if b[0] == '{' {
        s.headerMode = false
        dec := json.NewDecoder(s.r)
        var req JSONRPCRequest
        if err := dec.Decode(&req); err != nil {
            return nil, err
        }
        return &req, nil
    }
    // LSP-style header framing
    s.headerMode = true
    var contentLength int
    for {
        line, err := s.r.ReadString('\n')
        if err != nil {
            return nil, err
        }
        line = strings.TrimRight(line, "\r\n")
        if line == "" {
            break
        }
        if idx := strings.Index(line, ":"); idx >= 0 {
            key := strings.ToLower(strings.TrimSpace(line[:idx]))
            val := strings.TrimSpace(line[idx+1:])
            if key == "content-length" {
                if n, err := strconv.Atoi(val); err == nil {
                    contentLength = n
                }
            }
        }
    }
    if contentLength <= 0 {
        return nil, fmt.Errorf("invalid or missing Content-Length")
    }
    buf := make([]byte, contentLength)
    if _, err := io.ReadFull(s.r, buf); err != nil {
        return nil, err
    }
    dec := json.NewDecoder(bytes.NewReader(buf))
    var req JSONRPCRequest
    if err := dec.Decode(&req); err != nil {
        return nil, err
    }
    return &req, nil
}

func (s *StdioRPC) Reply(id any, result any) error {
    if s.headerMode {
        var buf bytes.Buffer
        enc := json.NewEncoder(&buf)
        if err := enc.Encode(JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result}); err != nil {
            return err
        }
        b := buf.Bytes()
        if _, err := fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
            return err
        }
        _, err := s.w.Write(b)
        return err
    }
    return writeResp(s.w, id, result, nil)
}

func (s *StdioRPC) ReplyError(id any, code int, msg string, data any) error {
    if s.headerMode {
        var buf bytes.Buffer
        enc := json.NewEncoder(&buf)
        if err := enc.Encode(JSONRPCResponse{JSONRPC: "2.0", ID: id, Error: &JSONRPCErrorObj{Code: code, Message: msg, Data: data}}); err != nil {
            return err
        }
        b := buf.Bytes()
        if _, err := fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
            return err
        }
        _, err := s.w.Write(b)
        return err
    }
    return writeResp(s.w, id, nil, &JSONRPCErrorObj{Code: code, Message: msg, Data: data})
}

// Helper ID bila perlu
var rid int64

func nextID() int64 { return atomic.AddInt64(&rid, 1) }
