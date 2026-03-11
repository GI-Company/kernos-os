.PHONY: build dev clean

# === Kernos OS Build System ===
# Compiles the React frontend + Go kernel into a single distributable binary.

BINARY_NAME := kernos
SERVER_DIR  := server
DIST_DIR    := $(SERVER_DIR)/dist

# Production build: single binary
build: frontend backend
	@echo "✅ Built: $(BINARY_NAME)"
	@ls -lh $(BINARY_NAME)

frontend:
	@echo "📦 Building frontend..."
	npm run build
	@echo "✅ Frontend built → $(DIST_DIR)"

backend:
	@echo "🔨 Compiling Go binary with embedded frontend..."
	cd $(SERVER_DIR) && go build -o ../$(BINARY_NAME) .
	@echo "✅ Backend compiled"

# Development: run both servers
dev:
	@echo "🚀 Starting dev mode..."
	@echo "   Frontend: http://localhost:3000"
	@echo "   Kernel:   ws://localhost:8080/ws"
	@trap 'kill 0' EXIT; \
		npm run dev & \
		cd $(SERVER_DIR) && go run . & \
		wait

# Clean build artifacts
clean:
	rm -rf $(DIST_DIR) $(BINARY_NAME)
	@echo "🧹 Cleaned"
