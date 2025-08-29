package httpserver

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	cfg "github.com/Rhyanz46/mcp-service/internal/config"
	"github.com/Rhyanz46/mcp-service/internal/ragvec"
)

type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// Start launches a simple HTTP server exposing similar functionality as MCP tools
func Start(addr string, conf *cfg.Config, rag *ragvec.VecRAG) {
	mux := http.NewServeMux()
	apiKey := strings.TrimSpace(conf.HTTP.APIKey)
	requireAuth := func(h http.HandlerFunc) http.HandlerFunc {
		if apiKey == "" {
			return h
		}
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(key), "bearer ") {
				key = strings.TrimSpace(key[7:])
			} else {
				key = r.Header.Get("X-API-Key")
			}
			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(errorResponse{Error: "unauthorized", Details: "Provide Authorization: Bearer <token> or X-API-Key header"})
				return
			}
			h(w, r)
		}
	}

	// health/status (fast by default)
	mux.HandleFunc("/status", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		fastOnly := true
		if v := r.URL.Query().Get("fast_only"); v != "" {
			if v == "0" || strings.EqualFold(v, "false") {
				fastOnly = false
			}
		}
		start := time.Now()
		q := ragvec.NewQdrantWithConfig(&conf.Qdrant, 1)
		healthErr := q.HealthCheck()
		var chunks *int
		if healthErr == nil {
			if c, err := q.CountPoints(); err == nil {
				chunks = &c
			}
		}
		var projectsCount *int
		var note string
		if healthErr == nil && !fastOnly {
			seen := map[string]struct{}{}
			var offset any
			for {
				pts, next, err := q.ScrollPoints(1000, offset)
				if err != nil {
					note = fmt.Sprintf("aggregation error: %v", err)
					break
				}
				for _, pt := range pts {
					if pth, ok := pt.Payload["path"].(string); ok {
						proj := projectFromPath(pth)
						seen[proj] = struct{}{}
					}
				}
				if next == nil {
					break
				}
				offset = next
				if time.Since(start) > 5*time.Second {
					note = "timeout: partial scan exceeded 5s"
					break
				}
			}
			if note == "" {
				v := len(seen)
				projectsCount = &v
			}
		} else if fastOnly {
			note = "fast_only=true"
		}
		status := map[string]any{
			"provider": conf.Embedding.Provider,
			"qdrant": map[string]any{
				"url":        conf.Qdrant.URL,
				"collection": conf.Qdrant.Collection,
				"health":     ifThenElse(healthErr == nil, "ok", safeErr(healthErr)),
			},
			"counts": map[string]any{
				"chunks":   chunks,
				"projects": projectsCount,
			},
			"config": map[string]any{
				"chunk_size":    conf.Indexing.ChunkSize,
				"chunk_overlap": conf.Indexing.ChunkOverlap,
				"batch_size":    conf.Indexing.BatchSize,
				"max_file_kb":   conf.Indexing.MaxFileKB,
				"exclude_dirs":  conf.Indexing.ExcludeDirs,
			},
			"degraded_mode": rag == nil,
			"fast_only":     fastOnly,
			"elapsed_ms":    time.Since(start).Milliseconds(),
			"note":          note,
		}
		writeJSON(w, http.StatusOK, status)
	}))

	// POST /rag/index {dir, include_code}
	mux.HandleFunc("/rag/index", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if rag == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "RAG not initialized", Details: "Start Qdrant or disable -no-qdrant"})
			return
		}
		var body struct {
			Dir         string `json:"dir"`
			IncludeCode bool   `json:"include_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json", Details: err.Error()})
			return
		}
		if strings.TrimSpace(body.Dir) == "" {
			body.Dir = "./docs"
		}
		n, err := rag.IngestDocs(body.Dir, body.IncludeCode)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "index error", Details: err.Error()})
			return
		}
		resp := map[string]any{
			"indexed":      n,
			"directory":    body.Dir,
			"include_code": body.IncludeCode,
			"status":       "success",
		}
		writeJSON(w, http.StatusOK, resp)
	}))

    // POST /rag/search {query, k, project, project_prefix}
    mux.HandleFunc("/rag/search", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if rag == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "RAG not initialized", Details: "Start Qdrant or disable -no-qdrant"})
			return
		}
		var body struct {
			Query         string `json:"query"`
			K             int    `json:"k"`
			Project       string `json:"project"`
			ProjectPrefix string `json:"project_prefix"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json", Details: err.Error()})
			return
		}
		if strings.TrimSpace(body.Query) == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query required"})
			return
		}
		if body.K <= 0 || body.K > 20 {
			body.K = 5
		}
		hits, err := rag.SearchWithFilter(body.Query, body.K, body.Project, body.ProjectPrefix)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "search error", Details: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"query": body.Query, "chunks": hits, "total_chunks": len(hits)})
    }))

    // POST /rag/delete {all, project}
    mux.HandleFunc("/rag/delete", requireAuth(func(w http.ResponseWriter, r *http.Request) {
        if rag == nil { writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "RAG not initialized", Details: "Start Qdrant or disable -no-qdrant"}); return }
        var body struct {
            All     bool   `json:"all"`
            Project string `json:"project"`
        }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid json", Details: err.Error()}); return }
        if !body.All && strings.TrimSpace(body.Project) == "" { writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid params", Details: "Provide all=true or a non-empty project"}); return }
        var del int
        var err error
        if body.All {
            del, err = rag.DeleteAll()
        } else {
            del, err = rag.DeleteProject(body.Project)
        }
        if err != nil { writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "delete error", Details: err.Error()}); return }
        writeJSON(w, http.StatusOK, map[string]any{"deleted": del, "all": body.All, "project": body.Project})
    }))

	// GET /rag/projects?prefix=&offset=&limit=
	mux.HandleFunc("/rag/projects", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if rag == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "RAG not initialized", Details: "Start Qdrant or disable -no-qdrant"})
			return
		}
		q := r.URL.Query()
		prefix := q.Get("prefix")
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		list, total, err := rag.ListProjectsFiltered(prefix, offset, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "projects error", Details: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": list, "count": len(list), "total": total, "offset": offset, "limit": limit, "filter": map[string]any{"prefix": prefix}})
	}))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("HTTP API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ifThenElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func safeErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func projectFromPath(p string) string {
	if p == "" {
		return "unknown"
	}
	// Derive project as basename of directory
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return "root"
	}
	rest := p[:idx]
	j := strings.LastIndex(rest, "/")
	if j < 0 {
		return rest
	}
	return rest[j+1:]
}
