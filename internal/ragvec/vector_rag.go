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
    "sort"
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

// HealthCheck verifies Qdrant is reachable by querying /collections
func (q *Qdrant) HealthCheck() error {
    url := fmt.Sprintf("%s/collections", q.baseURL)
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

// CountPoints returns the number of points in the current collection
func (q *Qdrant) CountPoints() (int, error) {
    url := fmt.Sprintf("%s/collections/%s/points/count", q.baseURL, q.collection)
    body := map[string]any{"exact": true}
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 10 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return 0, err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return 0, fmt.Errorf("count http %d", res.StatusCode)
    }
    var rr struct {
        Result struct {
            Count int `json:"count"`
        } `json:"result"`
    }
    if err := json.NewDecoder(res.Body).Decode(&rr); err != nil {
        return 0, err
    }
    return rr.Result.Count, nil
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

// ---------- Scrolling and project listing ----------
type ScrollPoint struct {
    ID      any            `json:"id"`
    Payload map[string]any `json:"payload"`
}

func (q *Qdrant) ScrollPoints(limit int, offset any) ([]ScrollPoint, any, error) {
    if limit <= 0 || limit > 10000 {
        limit = 1000
    }
    body := map[string]any{
        "limit":        limit,
        "with_payload": true,
    }
    if offset != nil {
        body["offset"] = offset
    }
    b, _ := json.Marshal(body)
    url := fmt.Sprintf("%s/collections/%s/points/scroll", q.baseURL, q.collection)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 15 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        return nil, nil, err
    }
    defer res.Body.Close()
    if res.StatusCode >= 300 {
        return nil, nil, fmt.Errorf("scroll http %d", res.StatusCode)
    }
    var rr struct {
        Result struct {
            Points         []struct {
                ID      any            `json:"id"`
                Payload map[string]any `json:"payload"`
            } `json:"points"`
            NextPageOffset any `json:"next_page_offset"`
        } `json:"result"`
    }
    if err := json.NewDecoder(res.Body).Decode(&rr); err != nil {
        return nil, nil, err
    }
    pts := make([]ScrollPoint, len(rr.Result.Points))
    for i, p := range rr.Result.Points {
        pts[i] = ScrollPoint{ID: p.ID, Payload: p.Payload}
    }
    return pts, rr.Result.NextPageOffset, nil
}

// ListProjects aggregates indexed chunks by project (directory name of each file)
func (r *VecRAG) ListProjects() ([]map[string]any, error) {
    // Scroll through all points and group by project name derived from payload.path
    counts := map[string]int{}
    files := map[string]map[string]struct{}{}
    var offset any
    for {
        pts, next, err := r.vdb.ScrollPoints(1000, offset)
        if err != nil {
            return nil, err
        }
        for _, pt := range pts {
            p := pt.Payload
            pathVal := toStr(p["path"])
            project := projectFromPath(pathVal)
            counts[project]++
            if files[project] == nil {
                files[project] = map[string]struct{}{}
            }
            files[project][toStr(p["basename"])]= struct{}{}
        }
        if next == nil {
            break
        }
        offset = next
    }
    out := make([]map[string]any, 0, len(counts))
    for proj, n := range counts {
        out = append(out, map[string]any{
            "project":      proj,
            "total_chunks": n,
            "files":        len(files[proj]),
        })
    }
    // Optional: sort by name
    sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["project"]) < fmt.Sprint(out[j]["project"]) })
    return out, nil
}

func projectFromPath(p string) string {
    if p == "" {
        return "unknown"
    }
    dir := filepath.Dir(p)
    if dir == "." || dir == "/" {
        return "root"
    }
    return filepath.Base(dir)
}

// ListProjectsFiltered filters by name prefix and paginates results after aggregation.
// Note: This scans the whole collection to aggregate per-project counts.
func (r *VecRAG) ListProjectsFiltered(prefix string, offset, limit int) ([]map[string]any, int, error) {
    list, err := r.ListProjects()
    if err != nil {
        return nil, 0, err
    }
    // filter by prefix (case-insensitive)
    fprefix := strings.ToLower(strings.TrimSpace(prefix))
    filtered := list[:0]
    if fprefix == "" {
        filtered = list
    } else {
        for _, it := range list {
            pname := strings.ToLower(fmt.Sprint(it["project"]))
            if strings.HasPrefix(pname, fprefix) {
                filtered = append(filtered, it)
            }
        }
    }
    total := len(filtered)
    if offset < 0 {
        offset = 0
    }
    if limit <= 0 {
        limit = 50
    }
    if offset > total {
        return []map[string]any{}, total, nil
    }
    end := offset + limit
    if end > total {
        end = total
    }
    page := make([]map[string]any, end-offset)
    copy(page, filtered[offset:end])
    return page, total, nil
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
                "project":   projectFromPath(c.Path),
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
    return r.SearchWithFilter(query, k, "", "")
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

// SearchWithFilter supports optional project or projectPrefix filtering.
// If project is set, it uses a server-side Qdrant filter for exact match.
// If projectPrefix is set (and project empty), it fetches a larger set then filters client-side.
func (r *VecRAG) SearchWithFilter(query string, k int, project string, projectPrefix string) ([]map[string]any, error) {
    if k <= 0 {
        k = 5
    }
    vecs, err := r.embed.Embed([]string{query})
    if err != nil {
        return nil, err
    }
    // Build filter for exact project match
    var filter map[string]any
    if strings.TrimSpace(project) != "" {
        filter = map[string]any{
            "must": []map[string]any{
                {
                    "key":   "project",
                    "match": map[string]any{"value": project},
                },
            },
        }
    }
    // If prefix provided without exact project, pull a larger page and filter client-side
    limit := k
    if filter == nil && strings.TrimSpace(projectPrefix) != "" {
        if k < 20 {
            limit = 20
        }
        if limit < k*5 {
            limit = k * 5
        }
        if limit > 100 {
            limit = 100
        }
    }
    res, err := r.vdb.Search(vecs[0], limit, filter)
    if err != nil {
        return nil, err
    }
    // Map hits
    items := make([]map[string]any, 0, len(res))
    for _, h := range res {
        p := h.Payload
        it := map[string]any{
            "id":        fmt.Sprint(h.ID),
            "score":     h.Score,
            "path":      toStr(p["path"]),
            "basename":  toStr(p["basename"]),
            "position":  p["position"],
            "snippet":   toStr(p["preview"]),
            "file_type": toStr(p["file_type"]),
            "project":   toStr(p["project"]),
        }
        items = append(items, it)
    }
    // Client-side prefix filter if needed
    if filter == nil && strings.TrimSpace(projectPrefix) != "" {
        pref := strings.ToLower(strings.TrimSpace(projectPrefix))
        filtered := items[:0]
        for _, it := range items {
            if strings.HasPrefix(strings.ToLower(fmt.Sprint(it["project"])), pref) {
                filtered = append(filtered, it)
            }
        }
        items = filtered
    }
    // Trim to k
    if len(items) > k {
        items = items[:k]
    }
    return items, nil
}
