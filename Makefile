# Makefile for MCP RAG Service

.PHONY: build test clean run start-qdrant stop-qdrant logs help init-config run-test demo-projects demo-search demo-status test-http-auth \
        install install-linux-user install-linux-system uninstall-linux-user uninstall-linux-system

# Default target
help:
	@echo "MCP RAG Service - Available commands:"
	@echo ""
	@echo "  build         Build the service binary"
	@echo "  test          Run tests and setup verification"
	@echo "  run           Run the service (requires OPENAI_API_KEY)"
	@echo "  init-config   Create config.json from config.example.json if missing"
	@echo "  start-qdrant  Start Qdrant vector database"
	@echo "  stop-qdrant   Stop Qdrant vector database"
	@echo "  logs          Show Qdrant logs"
	@echo "  clean         Clean build artifacts"
	@echo "  setup         Complete setup (build + start Qdrant + test docs)"
	@echo "  demo-projects Show example JSON-RPC for rag_projects"
	@echo "  demo-search   Show example JSON-RPC for rag_search"
	@echo "  demo-status   Show example JSON-RPC for status_get"
	@echo "  test-http-auth  Run HTTP auth smoke tests (requires curl)"
	@echo "  run-http      Run service with HTTP API (:8080)"
	@echo "  install       Install the binary on Linux (user/system)"
	@echo "                 - make install MODE=user   # -> $$HOME/.local/bin"
	@echo "                 - make install MODE=system # -> /usr/local/bin (requires sudo)"
	@echo ""
	@echo "Environment variables:"
	@echo "  OPENAI_API_KEY     - Required for embeddings"
	@echo "  QDRANT_URL         - Qdrant server URL (default: http://localhost:6333)"
	@echo "  QDRANT_COLLECTION  - Collection name (default: mcp_rag)"

# Build the service
build:
	@echo "🔨 Building MCP RAG service..."
	go mod tidy
	go build -v -o mcp-service .
	@echo "✅ Build complete"

# Run comprehensive test
test: build
	@echo "🧪 Running test suite..."
	./test.sh

# Run the service
run: build
	@echo "🚀 Starting MCP RAG service..."
	@if [ ! -f config.json ]; then \
		echo "❌ config.json not found. Create one (e.g., cp config.example.json config.json) or run with -config <path>."; \
		exit 1; \
	fi
	@if [ -z "$(OPENAI_API_KEY)" ]; then \
		echo "ℹ️  OPENAI_API_KEY not set. Using local TF-IDF embeddings."; \
	fi
	./mcp-service -config config.json

.PHONY: run-test
run-test: build
	@echo "🧪 Starting MCP RAG service (test mode)..."
	@if [ ! -f test-config.json ]; then \
		echo "❌ test-config.json not found. Provide a test config or use -config."; \
		exit 1; \
	fi
	./mcp-service -config test-config.json

# Demo: list projects via JSON-RPC over stdio
demo-projects: build
	@if [ ! -f config.json ]; then \
		echo "❌ config.json not found. Run 'make init-config' and adjust values."; \
		exit 1; \
	fi
	@echo "💡 Ensure Qdrant is running and collection contains data (run rag_index first)."
	@printf '%s\n' \
	  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
	  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_projects","arguments":{"prefix":"","offset":0,"limit":10}}}' \
	  | ./mcp-service -config config.json

# ---- Installation (Linux only) ----
BIN_NAME ?= mcp-service
USER_BIN ?= $(HOME)/.local/bin
SYSTEM_BIN ?= /usr/local/bin

DETECTED_OS := $(shell uname -s)

ifeq ($(DETECTED_OS),Linux)
ifeq ($(MODE),system)
install: install-linux-system
else
install: install-linux-user
endif
else
install:
	@echo "❌ install currently supports Linux only"; exit 1
endif

install-linux-user: build
	@echo "📦 Installing $(BIN_NAME) to $(USER_BIN)"
	@mkdir -p "$(USER_BIN)"
	@install -m 0755 ./$(BIN_NAME) "$(USER_BIN)/$(BIN_NAME)"
	@echo "✅ Installed to $(USER_BIN)/$(BIN_NAME)"
	@echo "➡  Ensure $$HOME/.local/bin is in your PATH"

install-linux-system: build
	@echo "📦 Installing $(BIN_NAME) to $(SYSTEM_BIN) (may require sudo)"
	@install -m 0755 ./$(BIN_NAME) "$(SYSTEM_BIN)/$(BIN_NAME)" 2>/dev/null || \
		{ echo "🔑 Retrying with sudo..."; sudo install -m 0755 ./$(BIN_NAME) "$(SYSTEM_BIN)/$(BIN_NAME)"; }
	@echo "✅ Installed to $(SYSTEM_BIN)/$(BIN_NAME)"

uninstall-linux-user:
	@echo "🗑  Removing $(USER_BIN)/$(BIN_NAME)"
	@rm -f "$(USER_BIN)/$(BIN_NAME)"
	@echo "✅ Removed (if it existed)"

uninstall-linux-system:
	@echo "🗑  Removing $(SYSTEM_BIN)/$(BIN_NAME) (may require sudo)"
	@rm -f "$(SYSTEM_BIN)/$(BIN_NAME)" 2>/dev/null || { echo "🔑 Retrying with sudo..."; sudo rm -f "$(SYSTEM_BIN)/$(BIN_NAME)"; }
	@echo "✅ Removed (if it existed)"

# Demo: run a sample search
demo-search: build
	@if [ ! -f config.json ]; then \
		echo "❌ config.json not found. Run 'make init-config' and adjust values."; \
		exit 1; \
	fi
	@echo "💡 Example rag_search for query: 'getting started' (adjust as needed)."
	@printf '%s\n' \
	  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
	  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rag_search","arguments":{"query":"getting started","k":5}}}' \
	  | ./mcp-service -config config.json

# Demo: get server status (status_get)
demo-status: build
	@if [ ! -f config.json ]; then \
		echo "❌ config.json not found. Run 'make init-config' and adjust values."; \
		exit 1; \
	fi
	@echo "💡 Example status_get (fast_only=true). Use fast_only=false for deeper aggregation."
	@printf '%s\n' \
	  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
	  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"status_get","arguments":{"fast_only":true}}}' \
	  | ./mcp-service -config config.json

.PHONY: run-http
run-http: build
	@if [ ! -f config.json ]; then \
		echo "❌ config.json not found. Run 'make init-config' and adjust values."; \
		exit 1; \
	fi
	@echo "🌐 Starting with HTTP API at :8080"
	./mcp-service -config config.json -http :8080

.PHONY: test-http-auth
test-http-auth: build
	@if ! command -v curl >/dev/null 2>&1; then \
		echo "❌ curl is required for this test"; exit 1; \
	fi
	bash scripts/test_http_auth.sh

# Start Qdrant
start-qdrant:
	@echo "🚀 Starting Qdrant vector database..."
	docker-compose up -d
	@echo "⏳ Waiting for Qdrant to be ready..."
	@for i in $$(seq 1 30); do \
		if curl -s http://localhost:6333/health > /dev/null 2>&1; then \
			echo "✅ Qdrant is ready at http://localhost:6333"; \
			break; \
		fi; \
		sleep 1; \
		if [ $$i -eq 30 ]; then \
			echo "❌ Qdrant failed to start"; \
			exit 1; \
		fi; \
	done

# Stop Qdrant
stop-qdrant:
	@echo "🛑 Stopping Qdrant..."
	docker-compose down

# Show Qdrant logs
logs:
	@echo "📋 Qdrant logs:"
	docker-compose logs -f qdrant

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f mcp-service
	go clean

# Complete setup
setup: build start-qdrant
	@echo "📝 Creating test documents..."
	@mkdir -p docs
	@echo "This is a test document about machine learning and AI." > docs/ml.txt
	@echo "# Vector Databases\n\nVector databases store high-dimensional vectors for similarity search." > docs/vectors.md
	@echo "✅ Setup complete! Run 'make run' to start the service."

# Initialize config.json from example
init-config:
	@if [ -f config.json ]; then \
		echo "✅ config.json already exists"; \
	else \
		if [ -f config.example.json ]; then \
			cp config.example.json config.json; \
			echo "✅ Created config.json from config.example.json"; \
		else \
			echo "❌ config.example.json not found. Please provide a config file."; \
			exit 1; \
		fi; \
	fi

# Development helpers
dev-test: build
	@echo "🔍 Quick development test..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./mcp-service &
	@sleep 1
	@echo "Test completed"

.DEFAULT_GOAL := help
