# loadout
#
# Release builds are plain `go build` with GOOS/GOARCH rather than a release
# tool, because there is no cgo anywhere and nothing needs a build container.
# One less dependency to keep working.

BINARY  := loadout
PKG     := ./cmd/loadout
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)
DIST    := dist

# os/arch pairs published for each release.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk -F':.*?## ' '{printf "  %-12s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build for the current platform into ./loadout
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

.PHONY: install
install: ## Build and install into ~/.local/bin
	go build -ldflags "$(LDFLAGS)" -o $(HOME)/.local/bin/$(BINARY) $(PKG)
	@echo "installed $(HOME)/.local/bin/$(BINARY) ($(VERSION))"

.PHONY: test
test: ## Run the Go tests
	go test ./...

.PHONY: check
check: ## Everything CI would run
	go build ./...
	go vet ./...
	go test ./...
	./bin/fmt --check
	@echo "all checks passed"

.PHONY: parity
parity: build ## Prove the generator still reproduces the original shell files
	@tmp=$$(mktemp -d); \
	mkdir -p $$tmp/xdg/loadout; \
	cp testdata/parity/config.yaml $$tmp/xdg/loadout/config.yaml; \
	XDG_CONFIG_HOME=$$tmp/xdg ./$(BINARY) apply --platform linux >/dev/null; \
	./testdata/parity/verify.sh $$tmp; \
	rm -rf $$tmp

.PHONY: release
release: ## Cross-compile every platform into ./dist with checksums
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out=$(DIST)/$(BINARY)_$${os}_$${arch}; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -ldflags "$(LDFLAGS)" -o $$out/$(BINARY) $(PKG) || exit 1; \
		cp LICENSE README.md $$out/ 2>/dev/null || true; \
		tar -czf $(DIST)/$(BINARY)_$(VERSION)_$${os}_$${arch}.tar.gz -C $$out .; \
		rm -rf $$out; \
	done
	@cd $(DIST) && shasum -a 256 *.tar.gz > checksums.txt 2>/dev/null || \
		(cd $(DIST) && sha256sum *.tar.gz > checksums.txt)
	@echo
	@ls -lh $(DIST)/*.tar.gz | awk '{print "  " $$9, $$5}'
	@echo
	@cat $(DIST)/checksums.txt

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(DIST) $(BINARY)
