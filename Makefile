# Makefile for MCP RAG Service

.PHONY: build test clean run start-qdrant stop-qdrant logs help

# Default target
help:
	@echo "MCP RAG Service - Available commands:"
	@echo ""
	@echo "  build         Build the service binary"
	@echo "  test          Run tests and setup verification"
	@echo "  run           Run the service (requires OPENAI_API_KEY)"
	@echo "  start-qdrant  Start Qdrant vector database"
	@echo "  stop-qdrant   Stop Qdrant vector database"
	@echo "  logs          Show Qdrant logs"
	@echo "  clean         Clean build artifacts"
	@echo "  setup         Complete setup (build + start Qdrant + test docs)"
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
	@if [ -z "$(OPENAI_API_KEY)" ]; then \
		echo "ℹ️  OPENAI_API_KEY not set. Using local TF-IDF embeddings."; \
	fi
	./mcp-service

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

# Development helpers
dev-test: build
	@echo "🔍 Quick development test..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./mcp-service &
	@sleep 1
	@echo "Test completed"

.DEFAULT_GOAL := help
