package chunker

import (
    "os"
    "path/filepath"
    "strings"

    cfg "github.com/Rhyanz46/mcp-service/internal/config"
)

type Chunk struct {
    ID       string // file:idx
    Path     string
    Text     string
    Position int
}

func readDocs(dir string, includeCode bool, config *cfg.Config) ([]struct{ Path, Text string }, error) {
    var out []struct{ Path, Text string }
    // Normalize base dir
    baseAbs, _ := filepath.Abs(dir)
    exclude := map[string]struct{}{}
    for _, d := range config.Indexing.ExcludeDirs {
        exclude[d] = struct{}{}
    }
    maxBytes := int64(config.Indexing.MaxFileKB) * 1024

    err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        // Skip symlinks unless allowed
        if (info.Mode() & os.ModeSymlink) != 0 && !config.Indexing.FollowSymlinks {
            if info.IsDir() { return filepath.SkipDir }
            return nil
        }
        // Skip excluded directories
        if info.IsDir() {
            name := filepath.Base(path)
            if _, ok := exclude[name]; ok {
                return filepath.SkipDir
            }
            return nil
        }
        // Guard: ensure path stays under base
        if abs, _ := filepath.Abs(path); !strings.HasPrefix(abs, baseAbs+string(os.PathSeparator)) && abs != baseAbs {
            return nil
        }

        ext := strings.ToLower(filepath.Ext(path))

        // Documentation files - always include
        if config.IsDocumentationFile(ext) {
            // Size check before reading
            if maxBytes > 0 && info.Size() > maxBytes {
                return nil
            }
            b, err := os.ReadFile(path)
            if err != nil {
                return err
            }
            out = append(out, struct{ Path, Text string }{path, string(b)})
            return nil
        }

        // Code files - only if includeCode is true
        if includeCode && config.IsCodeFile(ext) {
            if maxBytes > 0 && info.Size() > maxBytes {
                return nil
            }
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

// MakeChunks creates chunks from files in dir using config rules
func MakeChunks(dir string, size, overlap int, includeCode bool, config *cfg.Config) ([]Chunk, error) {
    files, err := readDocs(dir, includeCode, config)
    if err != nil {
        return nil, err
    }
    var out []Chunk
    for _, f := range files {
        parts := chunkText(f.Text, size, overlap)
        for i, p := range parts {
            id := filepath.Base(f.Path) + ":" + intToStr(i)
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
