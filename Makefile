# RelayGate — single Go module (product under ./core)
.PHONY: build test validate vet fmt panel clean dist dist-clean

GO ?= go
BIN ?= bin/relaygate
CMD ?= ./core/cmd/relaygate
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat RELEASE 2>/dev/null || echo dev)
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
DIST_ROOT ?= dist
DIST_NAME ?= relaygate-$(VERSION)-$(GOOS)-$(GOARCH)
DIST_DIR ?= $(DIST_ROOT)/$(DIST_NAME)

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BIN) $(CMD)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

# Dev validate writes runtime under .runtime/ (not a source-tree data/).
validate: build
	@test -f .env || cp .env.example .env
	./$(BIN) setup --noninteractive
	./$(BIN) validate

panel: build
	./$(BIN) panel

# Precompiled release tree (what install.sh extracts onto the host).
# Packs versioned packaging/ + empty install-prefix data/ skeleton; setup seeds into DataDir.
dist: build
	rm -rf "$(DIST_DIR)"
	mkdir -p "$(DIST_DIR)/bin" "$(DIST_DIR)/data/envoy" "$(DIST_DIR)/data/firewall" \
		"$(DIST_DIR)/data/prometheus" "$(DIST_DIR)/data/backups" "$(DIST_DIR)/data/inventory"
	cp -a "$(BIN)" "$(DIST_DIR)/bin/relaygate"
	chmod 755 "$(DIST_DIR)/bin/relaygate"
	cp -a frontend "$(DIST_DIR)/"
	cp -a packaging "$(DIST_DIR)/"
	# 根级初始化模板（非运行态）
	cp -a .env.example resources.example.yaml \
		gateway-01.env.example gateway-02.env.example \
		gateways.env.example \
		install.sh "$(DIST_DIR)/"
	chmod 755 "$(DIST_DIR)/install.sh"
	printf '%s\n' "$(VERSION)" > "$(DIST_DIR)/RELEASE"
	# 便于 FindRoot / 文档；不含完整源码
	cp -a go.mod "$(DIST_DIR)/" 2>/dev/null || true
	mkdir -p "$(DIST_ROOT)"
	tar -C "$(DIST_ROOT)" -czf "$(DIST_ROOT)/$(DIST_NAME).tar.gz" "$(DIST_NAME)"
	(cd "$(DIST_ROOT)" && sha256sum "$(DIST_NAME).tar.gz" > "$(DIST_NAME).tar.gz.sha256")
	@echo "dist: $(DIST_ROOT)/$(DIST_NAME).tar.gz"
	@cat "$(DIST_ROOT)/$(DIST_NAME).tar.gz.sha256"

dist-clean:
	rm -rf "$(DIST_ROOT)"

clean: dist-clean
	rm -f $(BIN)
	rm -rf .runtime
