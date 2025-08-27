# MCP RAG Service

A Model Context Protocol (MCP) service that provides document retrieval capabilities using vector databases. **Works completely offline** with local embeddings - no external API keys required!

## 🌟 Key Features

- **Offline-First**: Uses local TF-IDF embeddings by default (no OpenAI API required)
- **Flexible Configuration**: Centralized config system with JSON files and environment variables
- **Multiple File Types**: Supports 20+ file extensions for documentation and code
- **MCP Protocol**: Full compliance with Model Context Protocol specification
- **Vector Search**: Semantic similarity search using Qdrant vector database
- **Document Chunking**: Intelligent text chunking with configurable size and overlap
- **Development Ready**: Perfect for ForgeCODE and other development workflows

## 🚀 Quick Start

### 1. Start Qdrant
```bash
docker run -p 6333:6333 qdrant/qdrant
```

### 2. Run the Service
```bash
# Using default configuration (local embeddings)
go run .

# Using custom configuration
go run . -config=config.example.json

# Using environment variables
EMBEDDING_PROVIDER=local DOCS_DIR=./my-docs go run .
```

### 3. Test with Sample Data
```bash
# Run the comprehensive test suite
./test.sh

# Run the no-API demo
./demo_no_api.sh
```

## 📋 Configuration

The service uses a centralized configuration system that supports:

1. **Default configuration** (works out of the box)
2. **JSON configuration files** (see `config.example.json`)
3. **Environment variable overrides**

### Configuration Options

```json
{
  "server": {
    "name": "mcp-rag-service",
    "version": "1.0.0"
  },
  "embedding": {
    "provider": "local",          // "local" or "openai"
    "openai": {
      "api_key": "",
      "model": "text-embedding-3-small",
      "dim": 1536
    },
    "local": {
      "dim": 300                  // TF-IDF dimension
    }
  },
  "qdrant": {
    "url": "http://localhost:6333",
    "collection": "mcp_rag"
  },
  "indexing": {
    "docs_dir": "./docs",
    "chunk_size": 800,
    "chunk_overlap": 100,
    "batch_size": 10,
    "include_code": false,
    "file_types": {
      "documentation": [".md", ".txt", ".rst", ".adoc"],
      "code": [".go", ".py", ".js", ".ts", "..."],
      "config": [".json", ".yaml", ".yml", "..."],
      "database": [".sql", ".ddl", ".dml"],
      "web": [".html", ".css", ".scss", "..."]
    }
  },
  "logging": {
    "level": "info",
    "prefix": "[MCP-RAG]"
  }
}
```

### Environment Variables

```bash
# Embedding configuration
EMBEDDING_PROVIDER=local        # or "openai"
OPENAI_API_KEY=your-key-here   # only if using OpenAI

# Qdrant configuration
QDRANT_URL=http://localhost:6333
QDRANT_COLLECTION=my_collection

# Indexing configuration
DOCS_DIR=./documents
LOG_LEVEL=debug
```

## 🔧 Available Tools

### `rag_index`
Index documents from a directory into the vector database.

**Parameters:**
- `dir` (string): Directory path containing documents to index
- `include_code` (boolean): Whether to include code files in indexing

**Example:**
```json
{
  "name": "rag_index",
  "arguments": {
    "dir": "./docs",
    "include_code": true
  }
}
```

### `rag_search`
Search for relevant document chunks using semantic similarity.

**Parameters:**
- `query` (string): Search query for finding relevant document chunks
- `k` (integer, 1-20): Number of most relevant document chunks to return

**Example:**
```json
{
  "name": "rag_search",
  "arguments": {
    "query": "machine learning algorithms",
    "k": 5
  }
}
```

## 🏗️ Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   LLM (Claude)  │◄──►│  MCP Service    │◄──►│     Qdrant      │
│                 │    │  (Retrieval)    │    │ Vector Database │
│ - Receives chunks    │ - Index docs    │    │ - Store vectors │
│ - Generates answer   │ - Search docs   │    │ - Similarity    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │ Embedding Model │
                       │ Local TF-IDF OR │
                       │ OpenAI (optional)│
                       └─────────────────┘
```

## 🎯 Embedding Options

### 1. Local TF-IDF (Default)
- ✅ **No setup required**
- ✅ **Completely offline**
- ✅ **No API costs**
- ✅ **Fast startup**
- ⚠️ Basic semantic understanding

### 2. OpenAI Embeddings (Optional)
- ✅ **Superior semantic understanding**
- ✅ **Better search quality**
- ❌ Requires API key and internet
- ❌ API costs apply

## 📁 Supported File Types

### Documentation
- `.md` - Markdown files
- `.txt` - Plain text files
- `.rst` - reStructuredText files
- `.adoc` - AsciiDoc files

### Code Files
- `.go`, `.py`, `.js`, `.ts`, `.java`, `.cpp`, `.c`, `.h`
- `.cs`, `.php`, `.rb`, `.rs`, `.scala`, `.kt`, `.swift`
- `.dart`, `.r`, `.m`, `.sh`, `.bat`, `.ps1`

### Configuration
- `.json`, `.yaml`, `.yml`, `.xml`, `.toml`, `.ini`, `.cfg`, `.conf`

### Database
- `.sql`, `.ddl`, `.dml`

### Web
- `.html`, `.css`, `.scss`, `.less`, `.jsx`, `.tsx`, `.vue`, `.svelte`

## 🔗 Claude Desktop Integration

See [INTEGRATION.md](INTEGRATION.md) for detailed setup instructions.

**Quick setup:**
```json
{
  "mcpServers": {
    "rag-service": {
      "command": "/path/to/mcp-service",
      "args": ["-config", "/path/to/config.json"],
      "env": {
        "EMBEDDING_PROVIDER": "local"
      }
    }
  }
}
```

## 🧪 Testing

### Run All Tests
```bash
./test.sh
```

### Demo Without API Keys
```bash
./demo_no_api.sh
```

### Manual Testing
```bash
# Build the service
go build .

# Start the service
./mcp-service

# In another terminal, test with JSON-RPC
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./mcp-service
```

## 🛠️ Development

### Prerequisites
- Go 1.23.3+
- Docker (for Qdrant)
- Make (optional, for build automation)

### Build
```bash
# Standard build
go build .

# Using Make
make build

# Run tests
make test

# Clean build artifacts
make clean
```

### Project Structure
```
mcp-service/
├── main.go              # Entry point and MCP protocol handling
├── config.go            # Centralized configuration system
├── vector_rag.go        # Vector RAG implementation with Qdrant
├── local_embeddings.go  # Local TF-IDF embedding provider
├── chunker.go           # Document chunking logic
├── rag.go               # Basic text-based RAG (fallback)
├── mcp.go               # MCP protocol structures
├── config.example.json  # Example configuration file
├── test.sh              # Comprehensive test suite
├── demo_no_api.sh       # Offline demo script
├── Makefile             # Build automation
└── docs/                # Documentation and examples
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite: `./test.sh`
6. Submit a pull request

## 📄 License

[Add your license here]

## 🆘 Support

- **Issues**: Report bugs and request features on GitHub
- **Documentation**: See `INTEGRATION.md` for Claude Desktop setup
- **Examples**: Check the `demo_no_api.sh` script for usage examples

---

**🎉 Ready to get started?** Run `./test.sh` to verify everything works, then check out `INTEGRATION.md` for Claude Desktop setup!