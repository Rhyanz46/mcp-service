package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Global configuration instance
var GlobalConfig *Config

// Config represents the complete configuration structure
type Config struct {
	Server    ServerConfig    `json:"server"`
	Embedding EmbeddingConfig `json:"embedding"`
	Qdrant    QdrantConfig    `json:"qdrant"`
	Indexing  IndexingConfig  `json:"indexing"`
	Logging   LoggingConfig   `json:"logging"`
}

type ServerConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type EmbeddingConfig struct {
	Provider string          `json:"provider"` // "openai" or "local"
	OpenAI   OpenAIConfig    `json:"openai"`
	Local    LocalEmbedding  `json:"local"`
}

type OpenAIConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	Dim    int    `json:"dim"`
}

type LocalEmbedding struct {
	Dim int `json:"dim"`
}

type QdrantConfig struct {
	URL        string `json:"url"`
	Collection string `json:"collection"`
}

type IndexingConfig struct {
	DocsDir      string         `json:"docs_dir"`
	ChunkSize    int            `json:"chunk_size"`
	ChunkOverlap int            `json:"chunk_overlap"`
	BatchSize    int            `json:"batch_size"`
	IncludeCode  bool           `json:"include_code"`
	FileTypes    FileTypesConfig `json:"file_types"`
}

type FileTypesConfig struct {
	Documentation []string `json:"documentation"`
	Code          []string `json:"code"`
	Config        []string `json:"config"`
	Database      []string `json:"database"`
	Web           []string `json:"web"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Prefix string `json:"prefix"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name:    "mcp-rag-service",
			Version: "1.0.0",
		},
		Embedding: EmbeddingConfig{
			Provider: "local", // Default to local to avoid API dependencies
			OpenAI: OpenAIConfig{
				APIKey: os.Getenv("OPENAI_API_KEY"),
				Model:  "text-embedding-3-small",
				Dim:    1536,
			},
			Local: LocalEmbedding{
				Dim: 300, // TF-IDF dimension
			},
		},
		Qdrant: QdrantConfig{
			URL:        "http://localhost:6333",
			Collection: "mcp_rag",
		},
		Indexing: IndexingConfig{
			DocsDir:      "./docs",
			ChunkSize:    800,
			ChunkOverlap: 100,
			BatchSize:    10,
			IncludeCode:  false,
			FileTypes: FileTypesConfig{
				Documentation: []string{".md", ".txt", ".rst", ".adoc"},
				Code:          []string{".go", ".py", ".js", ".ts", ".java", ".cpp", ".c", ".h", ".cs", ".php", ".rb", ".rs", ".scala", ".kt", ".swift", ".dart", ".r", ".m", ".sh", ".bat", ".ps1"},
				Config:        []string{".json", ".yaml", ".yml", ".xml", ".toml", ".ini", ".cfg", ".conf"},
				Database:      []string{".sql", ".ddl", ".dml"},
				Web:           []string{".html", ".css", ".scss", ".less", ".jsx", ".tsx", ".vue", ".svelte"},
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Prefix: "[MCP-RAG]",
		},
	}
}

// InitConfig initializes the global configuration
func InitConfig(configPath string) error {
	config := DefaultConfig()
	
	// Load from file if specified
	if configPath != "" {
		if err := config.LoadFromFile(configPath); err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}
	}
	
	// Override with environment variables
	config.LoadFromEnv()
	
	// Validate configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	GlobalConfig = config
	return nil
}

// LoadFromFile loads configuration from a JSON file
func (c *Config) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(data, c)
}

// LoadFromEnv overrides configuration with environment variables
func (c *Config) LoadFromEnv() {
	// Server config
	if v := os.Getenv("MCP_SERVER_NAME"); v != "" {
		c.Server.Name = v
	}
	if v := os.Getenv("MCP_SERVER_VERSION"); v != "" {
		c.Server.Version = v
	}
	
	// Embedding config
	if v := os.Getenv("EMBEDDING_PROVIDER"); v != "" {
		c.Embedding.Provider = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		c.Embedding.OpenAI.APIKey = v
	}
	if v := os.Getenv("OPENAI_EMBED_MODEL"); v != "" {
		c.Embedding.OpenAI.Model = v
	}
	
	// Qdrant config
	if v := os.Getenv("QDRANT_URL"); v != "" {
		c.Qdrant.URL = v
	}
	if v := os.Getenv("QDRANT_COLLECTION"); v != "" {
		c.Qdrant.Collection = v
	}
	
	// Indexing config
	if v := os.Getenv("DOCS_DIR"); v != "" {
		c.Indexing.DocsDir = v
	}
	
	// Logging config
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.Name == "" {
		return fmt.Errorf("server name cannot be empty")
	}
	
	if c.Embedding.Provider != "openai" && c.Embedding.Provider != "local" {
		return fmt.Errorf("embedding provider must be 'openai' or 'local'")
	}
	
	if c.Embedding.Provider == "openai" && c.Embedding.OpenAI.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required when using OpenAI provider")
	}
	
	if c.Indexing.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	
	if c.Indexing.ChunkOverlap < 0 {
		return fmt.Errorf("chunk overlap cannot be negative")
	}
	
	if c.Indexing.BatchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}
	
	return nil
}

// IsDocumentationFile checks if the file extension is a documentation file
func (c *Config) IsDocumentationFile(ext string) bool {
	ext = strings.ToLower(ext)
	for _, docExt := range c.Indexing.FileTypes.Documentation {
		if ext == docExt {
			return true
		}
	}
	return false
}

// IsCodeFile checks if the file extension is a code file
func (c *Config) IsCodeFile(ext string) bool {
	ext = strings.ToLower(ext)
	for _, codeExt := range c.Indexing.FileTypes.Code {
		if ext == codeExt {
			return true
		}
	}
	return false
}

// GetFileType returns the type of file based on its extension
func (c *Config) GetFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	
	if c.IsDocumentationFile(ext) {
		return "documentation"
	}
	if c.IsCodeFile(ext) {
		return "code"
	}
	
	// Check other types
	for _, configExt := range c.Indexing.FileTypes.Config {
		if ext == configExt {
			return "config"
		}
	}
	for _, dbExt := range c.Indexing.FileTypes.Database {
		if ext == dbExt {
			return "database"
		}
	}
	for _, webExt := range c.Indexing.FileTypes.Web {
		if ext == webExt {
			return "web"
		}
	}
	
	return "other"
}

// SaveToFile saves the current configuration to a JSON file
func (c *Config) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}