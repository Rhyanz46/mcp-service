package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "strings"

    cfg "github.com/Rhyanz46/mcp-service/internal/config"
    "github.com/Rhyanz46/mcp-service/internal/mcp"
    "github.com/Rhyanz46/mcp-service/internal/ragvec"
)

func main() {
    // Parse command line flags
    var configPath string
    flag.StringVar(&configPath, "config", "", "Path to configuration file (optional)")
    flag.Parse()

    // Initialize configuration
    if err := cfg.InitConfig(configPath); err != nil {
        log.Fatalf("Failed to initialize config: %v", err)
    }

    // If no explicit config was provided, check for local config.json presence and inform
    if strings.TrimSpace(configPath) == "" {
        if _, err := os.Stat("config.json"); os.IsNotExist(err) {
            log.Println("No config.json found; using built-in defaults and environment overrides")
        }
    }

    // Setup logging based on config
    log.SetOutput(os.Stderr)
    log.SetPrefix(cfg.Global.Logging.Prefix + " ")

    log.Printf("Starting %s v%s...", cfg.Global.Server.Name, cfg.Global.Server.Version)
    log.Printf("Using embedding provider: %s", cfg.Global.Embedding.Provider)
    log.Printf("Qdrant URL: %s", cfg.Global.Qdrant.URL)
    log.Printf("Collection: %s", cfg.Global.Qdrant.Collection)

    rpc := mcp.NewStdioRPC()

    // Check Qdrant health quickly; if unavailable, continue without RAG
    var rag *ragvec.VecRAG
    q := ragvec.NewQdrantWithConfig(&cfg.Global.Qdrant, 1)
    if err := q.HealthCheck(); err != nil {
        log.Printf("Qdrant health check failed: %v", err)
        log.Println("Continuing startup; RAG tools will be unavailable until Qdrant is running")
    } else {
        // Init RAG vector with config
        var err error
        rag, err = ragvec.NewVecRAGWithConfig(cfg.Global)
        if err != nil {
            // Allow initialize/tools/list; show error details when tools are called
            log.Printf("RAG initialization warning: %v", err)
            log.Println("Service will start but RAG tools will be unavailable until Qdrant is running")
        } else {
            log.Println("RAG system initialized successfully")
        }
    }

	log.Println("MCP service ready, waiting for requests...")

	for {
		req, err := rpc.Read()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				log.Println("Client disconnected, shutting down...")
				return
			}
			log.Printf("Parse error: %v", err)
			_ = rpc.ReplyError(nil, -32700, "parse error", err.Error())
			return
		}

        if cfg.Global.Logging.Level == "debug" {
            log.Printf("Received request: %s", req.Method)
        }

		switch req.Method {
        case "initialize":
            res := mcp.InitializeResult{
                ProtocolVersion: "2024-11-05",
                Capabilities:    mcp.Capabilities{Tools: true},
                ServerInfo:      mcp.MCPServerInfo{Name: cfg.Global.Server.Name, Version: cfg.Global.Server.Version},
            }
            log.Println("Initialization completed")
            _ = rpc.Reply(req.ID, res)

        case "tools/list":
            tools := []mcp.Tool{
                {
                    Name:        "rag_index",
                    Description: fmt.Sprintf("Index documents from a directory into Qdrant vector database. Supports documentation (%v) and code files (%v).", cfg.Global.Indexing.FileTypes.Documentation, cfg.Global.Indexing.FileTypes.Code),
                    InputSchema: map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "dir": map[string]any{
                                "type":        "string",
                                "description": "Directory path containing documents to index",
                                "default":     "./docs",
                            },
                            "include_code": map[string]any{
                                "type":        "boolean",
                                "description": "Whether to include code files in indexing",
                                "default":     false,
                            },
                        },
                    },
                },
                {
                    Name:        "rag_search",
                    Description: "Search for relevant document chunks using semantic similarity. Returns raw document chunks for the LLM to use as context.",
                    InputSchema: map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "query": map[string]any{
                                "type":        "string",
                                "description": "Search query for finding relevant document chunks",
                            },
                            "k": map[string]any{
                                "type":        "integer",
                                "minimum":     1,
                                "maximum":     20,
                                "default":     5,
                                "description": "Number of most relevant document chunks to return",
                            },
                        },
                        "required": []string{"query"},
                    },
                },
            }
            if cfg.Global.Logging.Level == "debug" {
                log.Printf("Returning %d available tools", len(tools))
            }
            _ = rpc.Reply(req.ID, mcp.ToolsListResult{Tools: tools})

        case "tools/call":
            var p mcp.ToolsCallParams
            if err := json.Unmarshal(req.Params, &p); err != nil {
                log.Printf("Invalid tool call params: %v", err)
                _ = rpc.ReplyError(req.ID, -32602, "invalid params", err.Error())
                continue
            }

            if cfg.Global.Logging.Level == "debug" {
                log.Printf("Calling tool: %s", p.Name)
            }
			
			switch p.Name {
            case "rag_index":
                if rag == nil {
                    log.Println("RAG index requested but RAG system not initialized")
                    _ = rpc.ReplyError(req.ID, -32001, "RAG not initialized", 
                        "Please ensure Qdrant vector database is running")
                    break
                }

                dir := "./docs"
                if v, ok := p.Args["dir"].(string); ok && strings.TrimSpace(v) != "" {
                    dir = v
                }

                includeCode := false
                if v, ok := p.Args["include_code"].(bool); ok {
                    includeCode = v
                }

                log.Printf("Starting document indexing from directory: %s (include_code: %v)", dir, includeCode)
                n, err := rag.IngestDocs(dir, includeCode)
                if err != nil {
                    log.Printf("Index error: %v", err)
                    _ = rpc.ReplyError(req.ID, -32002, "index error", err.Error())
                    break
                }

                log.Printf("Successfully indexed %d document chunks", n)
                _ = rpc.Reply(req.ID, mcp.ToolsCallResult{Content: map[string]any{
                    "indexed":      n,
                    "directory":    dir,
                    "include_code": includeCode,
                    "status":       "success",
                    "message":      fmt.Sprintf("Successfully indexed %d document chunks from %s", n, dir),
                    "config": map[string]any{
                        "chunk_size":    cfg.Global.Indexing.ChunkSize,
                        "chunk_overlap": cfg.Global.Indexing.ChunkOverlap,
                        "batch_size":    cfg.Global.Indexing.BatchSize,
                        "provider":      cfg.Global.Embedding.Provider,
                    },
                }})

            case "rag_search":
                if rag == nil {
                    log.Println("RAG search requested but RAG system not initialized")
                    _ = rpc.ReplyError(req.ID, -32001, "RAG not initialized", 
                        "Please ensure Qdrant vector database is running")
                    break
                }

                q, _ := p.Args["query"].(string)
                if strings.TrimSpace(q) == "" {
                    log.Println("Empty search query provided")
                    _ = rpc.ReplyError(req.ID, -32602, "query required", "Search query cannot be empty")
                    break
                }

                k := 5
                if vv, ok := p.Args["k"]; ok {
                    if f, ok := vv.(float64); ok && f >= 1 && f <= 20 {
                        k = int(f)
                    }
                }

                if cfg.Global.Logging.Level == "debug" {
                    log.Printf("Performing semantic search: query='%s', k=%d", q, k)
                }
                hits, err := rag.Search(q, k)
                if err != nil {
                    log.Printf("Search error: %v", err)
                    _ = rpc.ReplyError(req.ID, -32003, "search error", err.Error())
                    break
                }

                log.Printf("Search completed, returning %d document chunks for LLM context", len(hits))
                _ = rpc.Reply(req.ID, mcp.ToolsCallResult{Content: map[string]any{
                    "query":        q,
                    "chunks":       hits,
                    "total_chunks": len(hits),
                    "message":      fmt.Sprintf("Found %d relevant document chunks", len(hits)),
                    "config": map[string]any{
                        "provider": cfg.Global.Embedding.Provider,
                    },
                }})

			default:
				log.Printf("Unknown tool requested: %s", p.Name)
				_ = rpc.ReplyError(req.ID, -32601, "tool not found", p.Name)
			}

        case "notifications/initialized":
            if cfg.Global.Logging.Level == "debug" {
                log.Println("Client initialization notification received")
            }
            _ = rpc.Reply(req.ID, map[string]any{"ok": true})

		default:
			log.Printf("Unknown method: %s", req.Method)
			_ = rpc.ReplyError(req.ID, -32601, "method not found", req.Method)
		}
	}
}
