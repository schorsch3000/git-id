BINARY    := git-id
PKG       := ./cmd/git-id
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X git-id/internal/cli.version=$(VERSION)
BIN_DIR   := bin
DIST_DIR  := dist
PREFIX    ?= /usr/local

# GOOS/GOARCH pairs built by `make release`.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.DEFAULT_GOAL := build
.PHONY: build man test vet fmt-check release clean install help

help: ## show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | sed -E 's/^([a-z-]+):.*## (.*)/  \1\t\2/'

build: ## build bin/git-id for this machine
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) $(PKG)

man: ## generate the man pages into bin/man (git id --help shows git-id.1)
	go run ./tools/genman $(BIN_DIR)/man

vet: ## go vet
	go vet ./...

fmt-check: ## fail if any file is not gofmt-formatted
	@test -z "$$(gofmt -l .)" || { echo "not formatted:"; gofmt -l .; exit 1; }

test: ## run all tests (e2e tests need git, ssh and, for some, zsh/fish/bash-completion)
	go test ./...

# Cross-compiled, static (CGO off) binaries, one archive per platform
# (.tar.gz with the man pages, .zip for Windows) plus dist/SHA256SUMS.
# Needs tar, zip, sha256sum.
release: fmt-check vet test ## cross-build release archives into dist/
	@command -v zip >/dev/null || { echo "zip is required for the Windows archives"; exit 1; }
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)
	go run ./tools/genman $(DIST_DIR)/.man
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = windows ] && ext=".exe"; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		stage=$(DIST_DIR)/.stage/$$name; \
		echo "==> $$os/$$arch"; \
		mkdir -p $$stage; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $$stage/$(BINARY)$$ext $(PKG); \
		if [ "$$os" = windows ]; then \
			(cd $$stage && zip -q ../../$$name.zip $(BINARY)$$ext); \
		else \
			cp -r $(DIST_DIR)/.man $$stage/man; \
			tar -C $$stage -czf $(DIST_DIR)/$$name.tar.gz $(BINARY)$$ext man; \
		fi; \
		rm -rf $$stage; \
	done
	rm -rf $(DIST_DIR)/.stage $(DIST_DIR)/.man
	cd $(DIST_DIR) && sha256sum * > SHA256SUMS
	@echo; cat $(DIST_DIR)/SHA256SUMS

install: build man ## install git-id and its man pages into $(PREFIX) (so `git id` and `git id --help` work)
	install -d $(DESTDIR)$(PREFIX)/bin $(DESTDIR)$(PREFIX)/share/man/man1
	install -m 0755 $(BIN_DIR)/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	install -m 0644 $(BIN_DIR)/man/*.1 $(DESTDIR)$(PREFIX)/share/man/man1/

clean: ## remove build output (bin/ and dist/)
	rm -rf $(BIN_DIR) $(DIST_DIR)
