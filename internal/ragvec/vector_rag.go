package ragvec

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    cfg "github.com/Rhyanz46/mcp-service/internal/config"
    "github.com/Rhyanz46/mcp-service/internal/chunker"
)

const (
    DefaultCollection = "mcp_rag"
    DefaultDim        = 1536 // text-embedding-3-small
)

type EmbeddingProvider interface {
    Embed(texts []string) ([][]float32, error)
    Dim() int
}

// ---------- OpenAI Embeddings ----------
type OpenAIProvider struct {
    apiKey string
    model  string
    dim    int
}

func NewOpenAIProviderWithConfig(config *cfg.OpenAIConfig) *OpenAIProvider {
    return &OpenAIProvider{
        apiKey: config.APIKey,
        model:  config.Model,
        dim:    config.Dim,
    }
}

func NewOpenAIProvider() *OpenAIProvider {
    k := os.Getenv("OPENAI_API_KEY")
    if k == "" {
        return nil
    }
    model := os.Getenv("OPENAI_EMBED_MODEL")
    if model == "" {
        model = "text-embedding-3-small"
    }
    return &OpenAIProvider{apiKey: k, model: model, dim: DefaultDim}
}

func (p *OpenAIProvider) Dim() int { return p.dim }

func (p *OpenAIProvider) Embed(texts []string) ([][]float32, error) {
    type reqT struct {
        Model string   `json:"model"`
        Input []string `json:"input"`
    }
    body, _ := json.Marshal(reqT{Model: p.model, Input: texts})
    req, _ := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+p.apiKey)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 30 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return nil, fmt.Errorf("openai embeddings http %d", res.StatusCode)
    }
    var rr struct {
        Data []struct {
            Embedding []float32 `json:"embedding"`
        } `json:"data"`
    }
    if err := json.NewDecoder(res.Body).Decode(&rr); err != nil {
        return nil, err
    }
    out := make([][]float32, len(rr.Data))
    for i, d := range rr.Data {
        out[i] = d.Embedding
    }
    return out, nil
}

// ---------- Qdrant minimal client ----------
type Qdrant struct {
    baseURL    string
    collection string
    dim        int
}

func NewQdrantWithConfig(config *cfg.QdrantConfig, dim int) *Qdrant {
    return &Qdrant{
        baseURL:    strings.TrimRight(config.URL, "/"),
        collection: config.Collection,
        dim:        dim,
    }
}

func NewQdrant(dim int) *Qdrant {
    u := os.Getenv("QDRANT_URL")
    if u == "" {
        u = "http://localhost:6333"
    }
    coll := os.Getenv("QDRANT_COLLECTION")
    if coll == "" {
        coll = DefaultCollection
    }
    return &Qdrant{baseURL: strings.TrimRight(u, "/"), collection: coll, dim: dim}
}

func (q *Qdrant) EnsureCollection() error {
    // PUT /collections/{name}
    url := fmt.Sprintf("%s/collections/%s", q.baseURL, q.collection)
    body := map[string]any{
        "vectors": map[string]any{
            "size":     q.dim,
            "distance": "Cosine",
        },
    }
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 10 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 && res.StatusCode != 409 { // 409 = already exists (ok)
        return fmt.Errorf("ensure collection http %d", res.StatusCode)
    }
    return nil
}

// HealthCheck verifies Qdrant is reachable by querying /health
func (q *Qdrant) HealthCheck() error {
    url := fmt.Sprintf("%s/health", q.baseURL)
    client := &http.Client{Timeout: 5 * time.Second}
    res, err := client.Get(url)
    if err != nil {
        return err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return fmt.Errorf("health http %d", res.StatusCode)
    }
    return nil
}

func (q *Qdrant) UpsertPoints(ids []string, vecs [][]float32, payloads []map[string]any) error {
    if len(ids) != len(vecs) || len(ids) != len(payloads) {
        return errors.New("mismatch len")
    }
    points := make([]map[string]any, 0, len(ids))
    for i := range ids {
        points = append(points, map[string]any{
            "id":      ids[i],
            "vector":  vecs[i],
            "payload": payloads[i],
        })
    }
    body := map[string]any{"points": points}
    b, _ := json.Marshal(body)
    url := fmt.Sprintf("%s/collections/%s/points?wait=true", q.baseURL, q.collection)
    req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 30 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return fmt.Errorf("upsert http %d", res.StatusCode)
    }
    return nil
}

type SearchHit struct {
    ID      any            `json:"id"`
    Score   float32        `json:"score"`
    Payload map[string]any `json:"payload"`
}

func (q *Qdrant) Search(vec []float32, k int, filter map[string]any) ([]SearchHit, error) {
    body := map[string]any{
        "vector": vec,
        "limit":  k,
    }
    if filter != nil {
        body["filter"] = filter
    }
    b, _ := json.Marshal(body)
    url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 15 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return nil, fmt.Errorf("search http %d", res.StatusCode)
    }

    var rr struct {
        Result []struct {
            ID      any            `json:"id"`
            Score   float32        `json:"score"`
            Payload map[string]any `json:"payload"`
        } `json:"result"`
    }
    if err := json.NewDecoder(res.Body).Decode(&rr); err != nil {
        return nil, err
    }
    out := make([]SearchHit, len(rr.Result))
    for i, v := range rr.Result {
        out[i] = SearchHit{ID: v.ID, Score: v.Score, Payload: v.Payload}
    }
    return out, nil
}

// ---------- RAG ops ----------
type VecRAG struct {
    embed  EmbeddingProvider
    vdb    *Qdrant
    config *cfg.Config
}

func NewVecRAGWithConfig(config *cfg.Config) (*VecRAG, error) {
    // Create embedding provider based on config
    var prov EmbeddingProvider

    switch config.Embedding.Provider {
    case "openai":
        if config.Embedding.OpenAI.APIKey == "" {
            return nil, fmt.Errorf("OpenAI API key is required when using OpenAI provider")
        }
        prov = NewOpenAIProviderWithConfig(&config.Embedding.OpenAI)
        fmt.Fprintf(os.Stderr, "[MCP-RAG] Using OpenAI embeddings\n")
    case "local":
        prov = NewLocalEmbeddingProviderWithConfig(&config.Embedding.Local)
        fmt.Fprintf(os.Stderr, "[MCP-RAG] Using local TF-IDF embeddings (no external API required)\n")
    default:
        return nil, fmt.Errorf("unsupported embedding provider: %s", config.Embedding.Provider)
    }

    q := NewQdrantWithConfig(&config.Qdrant, prov.Dim())
    if err := q.EnsureCollection(); err != nil {
        return nil, fmt.Errorf("failed to connect to Qdrant or create collection: %w (ensure Qdrant is running on %s)", err, q.baseURL)
    }

    return &VecRAG{embed: prov, vdb: q, config: config}, nil
}

func NewVecRAG() (*VecRAG, error) {
    // Fallback for backward compatibility
    return NewVecRAGWithConfig(cfg.DefaultConfig())
}

func (r *VecRAG) IngestDocs(dir string, includeCode bool) (int, error) {
    chunks, err := chunker.MakeChunks(dir, r.config.Indexing.ChunkSize, r.config.Indexing.ChunkOverlap, includeCode, r.config)
    if err != nil {
        return 0, err
    }
    if len(chunks) == 0 {
        return 0, nil
    }

    // Use batch size from config
    batchSize := r.config.Indexing.BatchSize
    total := 0
    for i := 0; i < len(chunks); i += batchSize {
        j := i + batchSize
        if j > len(chunks) {
            j = len(chunks)
        }
        batch := chunks[i:j]
        texts := make([]string, len(batch))
        for k, c := range batch {
            texts[k] = c.Text
        }

        vecs, err := r.embed.Embed(texts)
        if err != nil {
            return total, err
        }
        ids := make([]string, len(batch))
        payloads := make([]map[string]any, len(batch))
        for k, c := range batch {
            ids[k] = c.ID
            payloads[k] = map[string]any{
                "path":      c.Path,
                "position":  c.Position,
                "basename":  filepath.Base(c.Path),
                "preview":   preview(c.Text, 240),
                "file_type": r.config.GetFileType(c.Path),
            }
        }
        if err := r.vdb.UpsertPoints(ids, vecs, payloads); err != nil {
            return total, err
        }
        total += len(batch)
    }
    return total, nil
}

func (r *VecRAG) Search(query string, k int) ([]map[string]any, error) {
    vecs, err := r.embed.Embed([]string{query})
    if err != nil {
        return nil, err
    }
    res, err := r.vdb.Search(vecs[0], k, nil)
    if err != nil {
        return nil, err
    }
    out := make([]map[string]any, len(res))
    for i, h := range res {
        p := h.Payload
        out[i] = map[string]any{
            "id":        fmt.Sprint(h.ID),
            "score":     h.Score,
            "path":      toStr(p["path"]),
            "basename":  toStr(p["basename"]),
            "position":  p["position"],
            "snippet":   toStr(p["preview"]),
            "file_type": toStr(p["file_type"]),
        }
    }
    return out, nil
}

func preview(s string, n int) string {
    rs := []rune(strings.TrimSpace(s))
    if len(rs) <= n {
        return string(rs)
    }
    return string(rs[:n]) + "…"
}

func toStr(v any) string {
    switch t := v.(type) {
    case string:
        return t
    default:
        return fmt.Sprint(v)
    }
}
