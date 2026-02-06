# Magabot Makefile

VERSION := $(shell cat VERSION 2>/dev/null || echo "0.1.0")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w \
	-X github.com/kusa/magabot/internal/version.Version=$(VERSION) \
	-X github.com/kusa/magabot/internal/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/kusa/magabot/internal/version.BuildTime=$(BUILD_TIME)"
PLATFORMS := linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all build clean test install uninstall release

all: build

# Build for current platform
build:
	@echo "Building magabot..."
	go build $(LDFLAGS) -o bin/magabot ./cmd/magabot
	@echo "✅ Built: bin/magabot"

# Build for production (smaller)
build-prod:
	CGO_ENABLED=1 go build $(LDFLAGS) -o bin/magabot ./cmd/magabot
	strip bin/magabot 2>/dev/null || true
	@ls -lh bin/magabot

# Install to system
install: build
	@echo "Installing to /usr/local/bin..."
	@sudo cp bin/magabot /usr/local/bin/magabot
	@sudo chmod +x /usr/local/bin/magabot
	@echo "✅ Installed: /usr/local/bin/magabot"
	@echo ""
	@echo "Run 'magabot setup' to configure"

# Install for current user only (no sudo)
install-user: build
	@mkdir -p ~/bin
	@cp bin/magabot ~/bin/magabot
	@chmod +x ~/bin/magabot
	@echo "✅ Installed: ~/bin/magabot"
	@echo "Make sure ~/bin is in your PATH"

# Uninstall
uninstall:
	@echo "Removing magabot..."
	@sudo rm -f /usr/local/bin/magabot
	@rm -f ~/bin/magabot
	@echo "✅ Removed binary"
	@echo ""
	@echo "To remove config: rm -rf ~/.magabot"

# Clean
clean:
	rm -rf bin/
	rm -rf dist/
	go clean

# Test
test:
	go test -v ./...

# Download dependencies
deps:
	go mod download
	go mod tidy

# Generate encryption key
genkey:
	@go run ./cmd/magabot genkey

# Build releases for all platforms
release: clean
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT="dist/magabot_$${OS}_$${ARCH}"; \
		echo "Building $$OS/$$ARCH..."; \
		GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=0 go build $(LDFLAGS) -o $$OUTPUT ./cmd/magabot; \
		tar -czf $$OUTPUT.tar.gz -C dist magabot_$${OS}_$${ARCH}; \
		rm $$OUTPUT; \
	done
	@echo "✅ Releases built in dist/"
	@ls -lh dist/

# Quick run (build and start)
run: build
	./bin/magabot start

# Show version
version:
	@echo $(VERSION)

# Help
help:
	@echo "Magabot - Lightweight secure multi-platform chatbot"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  build        Build for current platform"
	@echo "  install      Install to /usr/local/bin (requires sudo)"
	@echo "  install-user Install to ~/bin (no sudo)"
	@echo "  uninstall    Remove magabot binary"
	@echo "  clean        Clean build artifacts"
	@echo "  test         Run tests"
	@echo "  deps         Download dependencies"
	@echo "  release      Build releases for all platforms"
	@echo "  run          Build and start"
	@echo "  help         Show this help"
