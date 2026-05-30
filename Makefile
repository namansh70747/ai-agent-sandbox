GO ?= /usr/local/go/bin/go
POLICY ?= configs/policies.yaml

.PHONY: all build tidy clean profile verify demo api api-dry test fmt vet verify-monitors

all: build

build:
	mkdir -p bin
	$(GO) build -o bin/sandbox-manager ./cmd/sandbox-manager
	$(GO) build -o bin/mcp-server      ./cmd/mcp-server
	$(GO) build -o bin/api-server      ./cmd/api-server

tidy:
	$(GO) mod tidy

# Unit tests (no urunc needed)
test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# Print per-tool isolation profiles from the policy (no containers needed)
profile:
	$(GO) run ./cmd/sandbox-manager --profile --policy $(POLICY)

# Verify urunc installation
verify:
	sudo $(GO) run ./cmd/sandbox-manager --verify

# Detect which urunc monitors can actually boot a Linux tool image here.
# Honest probe: runs `echo` under each monitor via a hypervisor annotation.
verify-monitors:
	@echo "== QEMU =="; \
	sudo nerdctl run --rm --runtime io.containerd.urunc.v2 \
	  --annotation com.urunc.unikernel.hypervisor=qemu \
	  localhost/ai-sandbox/base-tool:latest echo "qemu-ok" || echo "QEMU: FAILED"; \
	echo "== Firecracker =="; \
	sudo nerdctl run --rm --runtime io.containerd.urunc.v2 \
	  --annotation com.urunc.unikernel.hypervisor=firecracker \
	  localhost/ai-sandbox/base-tool:latest echo "fc-ok" \
	  || echo "Firecracker: cannot boot this Linux image (expected per urunc docs; keep QEMU)"

# Run live demo (requires urunc)
demo:
	sudo $(GO) run ./cmd/sandbox-manager --demo --policy $(POLICY)

# Start the REST + SSE MCP server with the policy (requires urunc for execute)
api:
	./bin/api-server --addr :8080 --policy $(POLICY)

# Start without policy (built-in 4 tools)
api-dry:
	./bin/api-server --addr :8080

clean:
	rm -rf bin/
