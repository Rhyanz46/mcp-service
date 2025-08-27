package main

import (
	"os"
	"path/filepath"
	"strings"
)

type Chunk struct {
	ID       string // file:idx
	Path     string
	Text     string
	Position int
}

func readDocs(dir string, includeCode bool, config *Config) ([]struct{ Path, Text string }, error) {
	var out []struct{ Path, Text string }
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		
		// Documentation files - always include
		if config.IsDocumentationFile(ext) {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, struct{ Path, Text string }{path, string(b)})
			return nil
		}
		
		// Code files - only if includeCode is true
		if includeCode && config.IsCodeFile(ext) {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(b)
			if len(text) > 0 {
				out = append(out, struct{ Path, Text string }{path, text})
			}
		}
		
		return nil
	})
	return out, err
}

// Helper function to identify code files
func isCodeFile(ext string) bool {
	codeExts := map[string]bool{
		".go":   true,
		".py":   true,
		".js":   true,
		".ts":   true,
		".java": true,
		".cpp":  true,
		".c":    true,
		".h":    true,
		".cs":   true,
		".php":  true,
		".rb":   true,
		".rs":   true,
		".sql":  true,
		".json": true,
		".yaml": true,
		".yml":  true,
		".xml":  true,
		".html": true,
		".css":  true,
	}
	return codeExts[ext]
}

func chunkText(text string, size, overlap int) []string {
	if size <= 0 {
		size = 800
	}
	if overlap < 0 {
		overlap = 0
	}
	var chunks []string
	runes := []rune(text)
	n := len(runes)
	for start := 0; start < n; {
		end := start + size
		if end > n {
			end = n
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == n {
			break
		}
		start = end - overlap
		if start < 0 {
			start = 0
		}
	}
	return chunks
}

func makeChunks(dir string, size, overlap int, includeCode bool, config *Config) ([]Chunk, error) {
	files, err := readDocs(dir, includeCode, config)
	if err != nil {
		return nil, err
	}
	var out []Chunk
	for _, f := range files {
		parts := chunkText(f.Text, size, overlap)
		for i, p := range parts {
			id := filepath.Base(f.Path) + ":" + // aman unik per file
				intToStr(i)
			out = append(out, Chunk{
				ID:       id,
				Path:     f.Path,
				Text:     p,
				Position: i,
			})
		}
	}
	return out, nil
}

// Simple integer to string conversion
func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	
	digits := []rune{}
	for i > 0 {
		digits = append([]rune{rune('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
