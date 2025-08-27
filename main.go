package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	cfg "github.com/Rhyanz46/mcp-service/internal/config"
	"github.com/Rhyanz46/mcp-service/internal/mcp"
	"github.com/Rhyanz46/mcp-service/internal/ragvec"
)

func main() {
	// Parse command line flags
	var configPath string
	var testFlag bool
	flag.StringVar(&configPath, "config", "", "Path to configuration file (optional)")
	flag.BoolVar(&testFlag, "test", false, "Enable testing mode (prefers test-config.json)")
	flag.Parse()

	// Resolve configuration path
	testMode := testFlag || os.Getenv("TEST_MODE") == "1" || strings.ToLower(os.Getenv("APP_ENV")) == "test"
	effectiveConfigPath := strings.TrimSpace(configPath)
	if effectiveConfigPath == "" {
		// Choose default based on mode
		if testMode {
			effectiveConfigPath = "test-config.json"
		} else {
			effectiveConfigPath = "config.json"
		}
	}
	if _, err := os.Stat(effectiveConfigPath); os.IsNotExist(err) {
		log.Fatalf("Config file not found: %s. Create it with `make init-config` or pass -config <path> (see config.example.json)", effectiveConfigPath)
	} else {
		log.Printf("Loading configuration from %s", effectiveConfigPath)
	}

	// Initialize configuration
	if err := cfg.InitConfig(effectiveConfigPath); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Setup logging based on config
	log.SetOutput(os.Stderr)
	log.SetPrefix(cfg.Global.Logging.Prefix + " ")

	log.Printf("Starting %s v%s...", cfg.Global.Server.Name, cfg.Global.Server.Version)
	log.Printf("Using embedding provider: %s", cfg.Global.Embedding.Provider)
	log.Printf("Qdrant URL: %s", cfg.Global.Qdrant.URL)
	log.Printf("Collection: %s", cfg.Global.Qdrant.Collection)

	rpc := mcp.NewStdioRPC()

	// Check Qdrant health with retry; fail after 5 attempts
	var rag *ragvec.VecRAG
	q := ragvec.NewQdrantWithConfig(&cfg.Global.Qdrant, 1)
	var healthErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := q.HealthCheck(); err != nil {
			healthErr = err
			log.Printf("Qdrant health check failed (attempt %d/5): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		healthErr = nil
		break
	}
	if healthErr != nil {
		log.Fatalf("Qdrant is not reachable after 5 attempts. Last error: %v", healthErr)
	}

	// Init RAG vector with config
	var err error
	rag, err = ragvec.NewVecRAGWithConfig(cfg.Global)
	if err != nil {
		log.Fatalf("Failed to initialize RAG: %v", err)
	}
	log.Println("RAG system initialized successfully")

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
                    Description: "Search for relevant document chunks using semantic similarity. Supports optional project filter.",
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
                            "project": map[string]any{
                                "type":        "string",
                                "description": "Filter results to an exact project name (parent folder)",
                                "default":     "",
                            },
                            "project_prefix": map[string]any{
                                "type":        "string",
                                "description": "Filter results to projects starting with this prefix (client-side)",
                                "default":     "",
                            },
                        },
                        "required": []string{"query"},
                    },
                },
                {
                    Name:        "rag_projects",
                    Description: "List detected projects (by parent directory) with total indexed chunks and file count. Supports prefix filter and pagination.",
                    InputSchema: map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "prefix": map[string]any{
                                "type":        "string",
                                "description": "Filter project names by prefix (case-insensitive)",
                                "default":     "",
                            },
                            "offset": map[string]any{
                                "type":        "integer",
                                "minimum":     0,
                                "default":     0,
                                "description": "Pagination offset",
                            },
                            "limit": map[string]any{
                                "type":        "integer",
                                "minimum":     1,
                                "maximum":     1000,
                                "default":     50,
                                "description": "Max number of projects to return",
                            },
                        },
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

                proj, _ := p.Args["project"].(string)
                projPref, _ := p.Args["project_prefix"].(string)
                if cfg.Global.Logging.Level == "debug" {
                    log.Printf("Performing semantic search: query='%s', k=%d, project='%s', project_prefix='%s'", q, k, proj, projPref)
                }
                hits, err := rag.SearchWithFilter(q, k, proj, projPref)
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
                        "provider":        cfg.Global.Embedding.Provider,
                        "project":         proj,
                        "project_prefix":  projPref,
                    },
                }})

            case "rag_projects":
                if rag == nil {
                    log.Println("RAG projects requested but RAG system not initialized")
                    _ = rpc.ReplyError(req.ID, -32001, "RAG not initialized", "Ensure Qdrant is running")
                    break
                }
                // Parse args
                var prefix string
                var offset, limit int
                if v, ok := p.Args["prefix"].(string); ok {
                    prefix = v
                }
                if v, ok := p.Args["offset"].(float64); ok {
                    if v >= 0 {
                        offset = int(v)
                    }
                }
                if v, ok := p.Args["limit"].(float64); ok {
                    if v >= 1 && v <= 1000 {
                        limit = int(v)
                    }
                }
                list, total, err := rag.ListProjectsFiltered(prefix, offset, limit)
                if err != nil {
                    log.Printf("Projects listing error: %v", err)
                    _ = rpc.ReplyError(req.ID, -32004, "projects error", err.Error())
                    break
                }
                _ = rpc.Reply(req.ID, mcp.ToolsCallResult{Content: map[string]any{
                    "projects": list,
                    "count":    len(list),
                    "total":    total,
                    "offset":   offset,
                    "limit":    limit,
                    "filter":   map[string]any{"prefix": prefix},
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
