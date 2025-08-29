package ragclassic

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Rhyanz46/mcp-service/internal/chunker"
	cfg "github.com/Rhyanz46/mcp-service/internal/config"
)

type Doc struct {
	ID    string
	Text  string
	Terms []string
}

type Inverted struct {
	Docs      []Doc
	DF        map[string]int            // document frequency
	TF        map[string]map[string]int // term -> docID -> tf
	DocLen    map[string]int
	AvgDocLen float64
	VocabSize int
	DocByID   map[string]Doc
	config    *cfg.Config
}

var wordRE = regexp.MustCompile(`[A-Za-z0-9_\p{L}]+`)

// Tokenize sederhana (lowercase + word char)
func tokenize(s string) []string {
	low := strings.ToLower(s)
	return wordRE.FindAllString(low, -1)
}

func loadDocsWithConfig(dir string, config *cfg.Config) ([]Doc, error) {
	// Use chunker to get documents
	chunks, err := chunker.MakeChunks(dir, config.Indexing.ChunkSize, config.Indexing.ChunkOverlap, config.Indexing.IncludeCode, config)
	if err != nil {
		return nil, err
	}
	var docs []Doc
	for _, chunk := range chunks {
		terms := tokenize(chunk.Text)
		docs = append(docs, Doc{ID: chunk.ID, Text: chunk.Text, Terms: terms})
	}
	return docs, nil
}

func buildIndex(docs []Doc, config *cfg.Config) *Inverted {
	idx := &Inverted{
		Docs:    docs,
		DF:      make(map[string]int),
		TF:      make(map[string]map[string]int),
		DocLen:  make(map[string]int),
		DocByID: make(map[string]Doc),
		config:  config,
	}
	totalLen := 0
	vocab := map[string]struct{}{}
	for _, d := range docs {
		idx.DocByID[d.ID] = d
		seen := map[string]bool{}
		for _, t := range d.Terms {
			if idx.TF[t] == nil {
				idx.TF[t] = make(map[string]int)
			}
			idx.TF[t][d.ID]++
			if !seen[t] {
				idx.DF[t]++
				seen[t] = true
			}
			vocab[t] = struct{}{}
		}
		idx.DocLen[d.ID] = len(d.Terms)
		totalLen += len(d.Terms)
	}
	if len(docs) > 0 {
		idx.AvgDocLen = float64(totalLen) / float64(len(docs))
	}
	idx.VocabSize = len(vocab)
	return idx
}

// BM25 sederhana
func (idx *Inverted) bm25Score(qTerms []string, docID string) float64 {
	const k1 = 1.5
	const b = 0.75
	N := float64(len(idx.Docs))
	score := 0.0
	docLen := float64(idx.DocLen[docID])
	for _, qt := range qTerms {
		df := float64(idx.DF[qt])
		if df == 0 {
			continue
		}
		idf := math.Log((N - df + 0.5) / (df + 0.5 + 1e-9))
		tf := float64(idx.TF[qt][docID])
		num := tf * (k1 + 1)
		den := tf + k1*(1-b+b*(docLen/idx.AvgDocLen))
		score += idf * (num / (den + 1e-9))
	}
	return score
}

// Cosine atas TF (fallback kecil untuk stabilitas)
func (idx *Inverted) cosineTF(qTerms []string, docID string) float64 {
	qtFreq := map[string]int{}
	for _, t := range qTerms {
		qtFreq[t]++
	}
	dt := idx.TF
	dtf := map[string]int{}
	for t := range qtFreq {
		if dt[t] != nil {
			dtf[t] = dt[t][docID]
		}
	}
	// dot
	dot := 0.0
	for t, qf := range qtFreq {
		dot += float64(qf * dtf[t])
	}
	// norms
	qn, dn := 0.0, 0.0
	for _, qf := range qtFreq {
		qn += float64(qf * qf)
	}
	for _, df := range dtf {
		dn += float64(df * df)
	}
	if qn == 0 || dn == 0 {
		return 0
	}
	return dot / (math.Sqrt(qn) * math.Sqrt(dn))
}

type Hit struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

func (idx *Inverted) Search(query string, k int) []Hit {
	q := tokenize(query)
	type pair struct {
		id string
		s  float64
	}
	var scores []pair
	// candidate docs
	cands := map[string]bool{}
	for _, t := range q {
		for docID := range idx.TF[t] {
			cands[docID] = true
		}
	}
	const alpha = 0.2
	for docID := range cands {
		b := idx.bm25Score(q, docID)
		c := idx.cosineTF(q, docID)
		s := b*(1-alpha) + c*alpha
		scores = append(scores, pair{docID, s})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].s > scores[j].s })
	if len(scores) > k {
		scores = scores[:k]
	}
	hits := make([]Hit, 0, len(scores))
	for _, p := range scores {
		snip := snippet(idx.DocByID[p.id].Text, q, 220)
		hits = append(hits, Hit{ID: p.id, Score: p.s, Snippet: snip})
	}
	return hits
}

func snippet(text string, q []string, max int) string {
	low := strings.ToLower(text)
	pos := -1
	for _, t := range q {
		if t == "" {
			continue
		}
		if i := strings.Index(low, t); i >= 0 {
			pos = i
			break
		}
	}
	if pos == -1 {
		if len(text) <= max {
			return text
		}
		return text[:max] + "…"
	}
	start := pos - max/3
	if start < 0 {
		start = 0
	}
	end := start + max
	if end > len(text) {
		end = len(text)
	}
	seg := text[start:end]
	for _, t := range q {
		if t == "" {
			continue
		}
		seg = strings.ReplaceAll(seg, t, fmt.Sprintf("**%s**", t))
		seg = strings.ReplaceAll(seg, strings.Title(t), fmt.Sprintf("**%s**", strings.Title(t)))
	}
	return seg
}

// Memuat dokumen dari config directory
func LoadIndexFromConfig(config *cfg.Config) (*Inverted, error) {
	dir := config.Indexing.DocsDir
	if dir == "" {
		dir = "./docs"
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
		// Seed contoh
		f, _ := os.Create(filepath.Join(dir, "welcome.txt"))
		w := bufio.NewWriter(f)
		fmt.Fprintln(w, "Ini contoh dokumen. Simpan file .txt atau .md lain di folder docs untuk diindeks.")
		w.Flush()
		f.Close()
	}
	docs, err := loadDocsWithConfig(dir, config)
	if err != nil {
		return nil, err
	}
	return buildIndex(docs, config), nil
}
