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
# Using default configuration (requires config.json)
cp -n config.example.json config.json  # if you don't have one
go run main.go -config=config.json

# Using custom configuration
go run . -config=config.example.json

# Using environment variables
EMBEDDING_PROVIDER=local DOCS_DIR=./my-docs go run main.go -config=config.json

# Testing mode (prefers test-config.json)
go run main.go -test
# or via env:
TEST_MODE=1 go run main.go
APP_ENV=test go run main.go
```

### Alternative: Makefile
```bash
make run            # requires config.json
make run-test       # uses test-config.json

# Degraded mode (for MCP discovery without Qdrant)
go run main.go -config=config.json -no-qdrant
MCP_NO_QDRANT=1 ./mcp-service -config config.json
```

## ✅ Startup Checks

- Config file: The app requires a config file. By default it expects `config.json`, and in testing mode it prefers `test-config.json`. You can override with `-config <path>`.
- If the chosen file is not found, startup fails with a clear error.
- Qdrant health: On startup, it pings `QDRANT_URL` and retries up to 5 times. If still unreachable, startup fails with an error.
  - For MCP clients that just need to list tools without Qdrant, run with `-no-qdrant` or env `MCP_NO_QDRANT=1`.

## 📦 Project Layout

- `main.go`: entrypoint wiring config, MCP, and RAG.
- `internal/config`: configuration types, env/file loaders, `config.Global`.
- `internal/mcp`: JSON-RPC and MCP request/response structures, stdio transport.
- `internal/chunker`: document scanning and chunking helpers.
- `internal/ragvec`: vector RAG with Qdrant + embeddings (default in main).
- `internal/ragclassic`: classic BM25/TF index (kept for reference).

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
    "max_file_kb": 1024,
    "exclude_dirs": [".git", "node_modules", "vendor", "build", "dist", "target", ".venv"],
    "follow_symlinks": false,
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

### `rag_projects`
List detected projects (grouped by parent directory of each indexed file) with total indexed chunks and number of distinct files.

Parameters:
- `prefix` (string, optional): Case-insensitive starts-with filter for project names. Default: empty (no filter).
- `offset` (integer, optional): Pagination offset. Default: 0.
- `limit` (integer, optional): Max number of projects to return. Default: 50. Max: 1000.

Examples:
```json
{
  "name": "rag_projects",
  "arguments": {}
}
```

```json
{
  "name": "rag_projects",
  "arguments": { "prefix": "doc" }
}
```

```json
{
  "name": "rag_projects",
  "arguments": { "offset": 50, "limit": 50 }
}
```

Response shape:
```json
{
  "projects": [
    { "project": "docs", "total_chunks": 128, "files": 6 },
    { "project": "api",  "total_chunks": 64,  "files": 3 }
  ],
  "count": 2,
  "total": 10,
  "offset": 0,
  "limit": 50,
  "filter": { "prefix": "doc" }
}
```

Notes:
- Project name is derived from the parent directory of each chunk's `payload.path`. Example: `./docs/readme.md` → project `docs`.
- This endpoint aggregates by scanning all points in the collection. For very large datasets, consider adding a `project` payload during ingestion and indexing it in Qdrant for faster aggregations.

### `status_get`
Dapatkan status server secara ringkas: provider embedding, kesehatan Qdrant, jumlah chunks, jumlah proyek (opsional), dan ringkasan konfigurasi indexing.

Parameters:
- `fast_only` (boolean, default: `true`): Jika `true`, hanya metrik cepat (health, total chunks via count). Jika `false`, server mencoba mengagregasi jumlah proyek dengan memindai koleksi (dapat memakan waktu pada dataset besar).

Example:
```json
{
  "name": "status_get",
  "arguments": { "fast_only": true }
}
```

Response shape (contoh):
```json
{
  "provider": "local",
  "qdrant": { "url": "http://localhost:6333", "collection": "mcp_rag", "health": "ok" },
  "counts": { "chunks": 1234, "projects": null },
  "config": { "chunk_size": 800, "chunk_overlap": 100, "batch_size": 10, "max_file_kb": 1024, "exclude_dirs": [".git","node_modules", "vendor", "build", "dist", "target", ".venv"] },
  "degraded_mode": false,
  "fast_only": true,
  "elapsed_ms": 21,
  "note": "fast_only=true"
}
```

## 🧪 Example Usage

### End-to-end: Index, then list projects
Ensure Qdrant is running and you have a valid `config.json`.

1) Index a directory (optional if already indexed):
```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_index","arguments":{"dir":"./docs","include_code":false}}}' \
  | ./mcp-service -config config.json
```

2) List projects (first 10 entries):
```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_projects","arguments":{"prefix":"","offset":0,"limit":10}}}' \
  | ./mcp-service -config config.json
```

3) Filter by prefix "doc":
```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_projects","arguments":{"prefix":"doc","limit":5}}}' \
  | ./mcp-service -config config.json
```

### Makefile shortcut
```bash
make demo-projects
```

### Search using rag_search
You can perform a semantic search across all indexed content. After inspecting projects via `rag_projects`, choose a relevant query and run `rag_search`.

Note: Current implementation searches globally (no per-project filter yet). You can add project keywords into your query to bias results or extend the tool to support project filtering.

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_search","arguments":{"query":"getting started","k":5}}}' \
  | ./mcp-service -config config.json
```

Makefile shortcut:
```bash
make demo-search
```

## 🌐 HTTP API (opsional)

Jalankan service dengan HTTP API:

```bash
go run . -config=config.json -http :8080
# atau binary:
./mcp-service -config config.json -http :8080
```

Endpoints:
- `GET /status?fast_only=true` – ringkasan status (mirip tool `status_get`).
- `POST /rag/index` – body: `{ "dir": "./docs", "include_code": false }`.
- `POST /rag/search` – body: `{ "query": "...", "k": 5, "project": "", "project_prefix": "" }`.
- `GET /rag/projects?prefix=&offset=&limit=` – daftar proyek terindeks.

Contoh:
```bash
curl -s http://localhost:8080/status | jq
curl -s -X POST http://localhost:8080/rag/index \
  -H 'content-type: application/json' \
  -d '{"dir":"./docs","include_code":false}' | jq
curl -s -X POST http://localhost:8080/rag/search \
  -H 'content-type: application/json' \
  -d '{"query":"getting started","k":5}' | jq
curl -s 'http://localhost:8080/rag/projects?prefix=&offset=0&limit=10' | jq
```

### Server status via status_get
Lihat ringkasan status server (provider, health, counts) untuk troubleshooting cepat.

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"status_get","arguments":{"fast_only":true}}}' \
  | ./mcp-service -config config.json
```

Makefile shortcut:
```bash
make demo-status
```

## 🛡️ Indexing Guardrails

Untuk mencegah pembacaan berkas yang tidak perlu atau terlalu besar saat `rag_index`:

- `max_file_kb` (default 1024): Berkas lebih besar dari nilai ini akan di-skip.
- `exclude_dirs`: Direktori yang tidak dipindai (default: `.git`, `node_modules`, `vendor`, `build`, `dist`, `target`, `.venv`).
- `follow_symlinks` (default false): Jika `false`, symlink akan di-skip; mengurangi risiko keluar dari root direktori.

Semua opsi dapat dikonfigurasi di `config.json` pada bagian `indexing`.

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

## 🔗 Gemini CLI Integration

The service is compatible with the Gemini CLI MCP client via stdio transport and JSON-RPC framing.

- Ensure the binary is built: `go build .`
- Verify Qdrant is running, or start in degraded mode with `-no-qdrant` for tool discovery only.
- The repo includes a `.gemini/settings.json` that points Gemini CLI to the local server.

Example `.gemini/settings.json` (already provided):
```json
{
  "mcpServers": {
    "rag-service": {
      "command": "./mcp-service",
      "args": ["-config", "./config.json"]
    }
  }
}
```

Notes:
- Initialization returns `protocolVersion` `2024-11-05` and advertises `tools` capability as an empty object (per spec).
- Tool results return `content` as an array with both a human-readable `text` item and a structured `json` item, which works well across MCP clients including Gemini CLI.
- For discovery without Qdrant, launch with `-no-qdrant` or `MCP_NO_QDRANT=1`.

### MCP Compliance Notes
- `capabilities.tools` harus berupa objek kosong `{}` untuk mengindikasikan dukungan tools (bukan boolean).
- `notifications/initialized` adalah JSON-RPC notification (tanpa `id`) — server tidak boleh membalas pesan ini.

### Troubleshooting (Gemini)
- Error `capabilities.tools` boolean: pastikan binary yang dipanggil Gemini adalah versi terbaru yang mengirim `{}`. Gunakan path absolut di `.gemini/settings.json`.
- Error Zod `unrecognized key(s) 'result'` pada `notifications/initialized`: terjadi jika server mengirim response untuk notification. Versi ini sudah memperbaikinya (no-reply). Pastikan Gemini menjalankan binary terbaru.

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
