GO ?= /usr/local/go/bin/go

.PHONY: all build tidy clean profile verify demo

all: build

build:
	mkdir -p bin
	$(GO) build -o bin/sandbox-manager ./cmd/sandbox-manager
	$(GO) build -o bin/mcp-server      ./cmd/mcp-server
	$(GO) build -o bin/api-server      ./cmd/api-server

tidy:
	$(GO) mod tidy

# Print per-tool isolation profiles (no containers needed)
profile:
	$(GO) run ./cmd/sandbox-manager --profile

# Verify urunc installation
verify:
	sudo $(GO) run ./cmd/sandbox-manager --verify

# Run live demo (requires urunc)
demo:
	sudo $(GO) run ./cmd/sandbox-manager --demo

# Start the REST + SSE MCP server (requires urunc for execute)
api:
	sudo ./bin/api-server --addr :8080

# Start the REST + SSE MCP server without sudo (profiles endpoint works)
api-dry:
	./bin/api-server --addr :8080

clean:
	rm -rf bin/
