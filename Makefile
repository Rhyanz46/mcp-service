# Makefile for MCP RAG Service

.PHONY: build test clean run start-qdrant stop-qdrant logs help init-config run-test demo-projects demo-search

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
