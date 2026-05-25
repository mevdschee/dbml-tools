# dbml-tools — Makefile
#
# Common targets:
#   make                   # native build → ./dbml-tools
#   make test              # run the Go test suite
#   make cross             # cross-compile for every VSCode-supported platform
#   make cross-archives    # cross + .tar.gz / .zip archives
#   make sync-vscode       # copy cross-built binaries into the sibling
#                          # ../dbml-tools-vscode/server-bin/ tree
#   make clean             # remove ./dbml-tools and ./dist
#
# Cross-compile is pure Go (CGO disabled). No C toolchain required.

GO        ?= go
BIN       ?= dbml-tools
DIST      ?= dist
VSCODE_DIR ?= ../dbml-tools-vscode
VERSION   ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)$(shell git diff --quiet 2>/dev/null && git diff --cached --quiet 2>/dev/null || echo -n -dirty)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS = -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

.PHONY: all build test cross cross-archives sync-vscode verify clean

all: build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .
	@./$(BIN) version

test:
	$(GO) test ./...

cross:
	./scripts/build-binaries.sh

cross-archives:
	./scripts/build-binaries.sh --archives

# Copy each cross-built binary into the extension repo's server-bin/ layout,
# which the extension reads via path.join(extensionPath, 'server-bin', `${process.platform}-${process.arch}`).
sync-vscode: cross
	@if [ ! -d "$(VSCODE_DIR)" ]; then \
		echo "vscode repo not found at $(VSCODE_DIR)"; \
		echo "set VSCODE_DIR=/path/to/dbml-tools-vscode"; \
		exit 1; \
	fi
	@for t in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do \
		src="$(DIST)/$$t/$(BIN)"; \
		ext=""; \
		case "$$t" in win32-*) ext=".exe" ;; esac; \
		src="$$src$$ext"; \
		dst_dir="$(VSCODE_DIR)/server-bin/$$t"; \
		mkdir -p "$$dst_dir"; \
		cp -f "$$src" "$$dst_dir/$(BIN)$$ext"; \
		echo "  copied $$t"; \
	done
	@echo "done — version: $(VERSION)"

# Verify the SHA256SUMS file produced by `make cross`.
verify:
	@if [ ! -f $(DIST)/SHA256SUMS ]; then \
		echo "no $(DIST)/SHA256SUMS — run 'make cross' first"; exit 1; \
	fi
	@(cd $(DIST) && sha256sum -c SHA256SUMS)

clean:
	rm -rf $(BIN) $(DIST)
