package ragvec

import (
	"crypto/md5"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	cfg "github.com/Rhyanz46/mcp-service/internal/config"
)

// Simple local embedding provider using TF-IDF
type LocalEmbeddingProvider struct {
	vocab     map[string]int
	idf       map[string]float64
	vocabSize int
	dim       int
}

func NewLocalEmbeddingProviderWithConfig(config *cfg.LocalEmbedding) *LocalEmbeddingProvider {
	return &LocalEmbeddingProvider{
		vocab: make(map[string]int),
		idf:   make(map[string]float64),
		dim:   config.Dim,
	}
}

func NewLocalEmbeddingProvider() *LocalEmbeddingProvider {
	return &LocalEmbeddingProvider{
		vocab: make(map[string]int),
		idf:   make(map[string]float64),
		dim:   512, // Fixed dimension for consistency
	}
}

func (p *LocalEmbeddingProvider) Dim() int { return p.dim }

// Build vocabulary and IDF from a corpus of texts
func (p *LocalEmbeddingProvider) BuildVocab(texts []string) {
	// Build vocabulary
	vocabSet := make(map[string]bool)
	docFreq := make(map[string]int)

	for _, text := range texts {
		terms := tokenizeText(text)
		seen := make(map[string]bool)
		for _, term := range terms {
			vocabSet[term] = true
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	// Convert to ordered vocab
	var vocabList []string
	for term := range vocabSet {
		vocabList = append(vocabList, term)
	}
	sort.Strings(vocabList)

	p.vocabSize = len(vocabList)
	for i, term := range vocabList {
		p.vocab[term] = i
	}

	// Calculate IDF
	totalDocs := float64(len(texts))
	for term, df := range docFreq {
		p.idf[term] = math.Log(totalDocs / (float64(df) + 1.0))
	}
}

func (p *LocalEmbeddingProvider) Embed(texts []string) ([][]float32, error) {
	if len(p.vocab) == 0 {
		// Build vocab from input texts if not already built
		p.BuildVocab(texts)
	}
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embeddings[i] = p.textToVector(text)
	}
	return embeddings, nil
}

func (p *LocalEmbeddingProvider) textToVector(text string) []float32 {
	terms := tokenizeText(text)

	// Calculate TF
	tf := make(map[string]float64)
	for _, term := range terms {
		tf[term]++
	}

	// Normalize TF
	totalTerms := float64(len(terms))
	for term := range tf {
		tf[term] = tf[term] / totalTerms
	}

	// Create sparse TF-IDF vector
	tfidf := make(map[int]float64)
	for term, tfVal := range tf {
		if idx, exists := p.vocab[term]; exists {
			idfVal := p.idf[term]
			tfidf[idx] = tfVal * idfVal
		}
	}

	// Convert to dense vector with fixed dimension
	vector := make([]float32, p.dim)

	// Hash-based dimensionality reduction
	for idx, val := range tfidf {
		// Use multiple hash functions to distribute features
		for h := 0; h < 3; h++ {
			hashInput := fmt.Sprintf("%d_%d", idx, h)
			hash := md5.Sum([]byte(hashInput))
			pos := int(hash[0])%p.dim + int(hash[1])%p.dim + int(hash[2])%p.dim
			pos = pos % p.dim
			vector[pos] += float32(val / 3.0) // Divide by number of hash functions
		}
	}

	// Normalize vector
	norm := float32(0)
	for _, v := range vector {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range vector {
			vector[i] = vector[i] / norm
		}
	}
	return vector
}

// Simple tokenizer
func tokenizeText(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)
	// Remove code-specific noise but keep meaningful terms
	text = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(text, " ")
	// Split on whitespace
	terms := regexp.MustCompile(`\s+`).Split(text, -1)
	// Filter out short terms and common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
	}
	var filtered []string
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if len(term) > 2 && !stopWords[term] {
			filtered = append(filtered, term)
		}
	}
	return filtered
}
