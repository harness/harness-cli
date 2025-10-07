# Makefile for generating REST clients + Cobra commands from all OpenAPI specs

API_DIR := api
SERVICES := $(shell find $(API_DIR) -mindepth 2 -maxdepth 2 -type f -name "openapi.yaml" | \
	            sed -E "s|$(API_DIR)/([^/]+)/openapi.yaml|\1|")

GOCMD := go
GEN   := ./tools/cobra-gen

ifndef GOPATH
	GOPATH := $(shell go env GOPATH)
endif
ifndef GOBIN # derive value from gopath (default to first entry, similar to 'go get')
	GOBIN := $(shell go env GOPATH | sed 's/:.*//')/bin
endif

.PHONY: generate build clean

# Generate code for every service folder that owns an openapi.yaml
generate: $(SERVICES:%=generate-%)


tools = $(addprefix $(GOBIN)/, golangci-lint goimports govulncheck protoc-gen-go protoc-gen-go-grpc gci)
tools: $(tools) ## Install tools required for the build
	@echo "Installed tools"


generate-%:
	@echo ">> Generating $*"
	$(GOCMD) run $(GEN) --service=$* --in=$(API_DIR)/$*/openapi.yaml --out=cmd/$* --cmd=$(API_DIR)/$*/command.yaml

# Build the CLI (runs `generate` first so everything is up to date)
build: generate format
	$(GOCMD) build -o hc ./cmd/hc

# Remove generated artifacts
clean:
	rm -rf internal/api/*
	rm -f  cmd/*/*_gen.go

format: tools # Format go code and error if any changes are made
	@echo "Formatting ..."
	@goimports -w .
	@gci write --skip-generated --custom-order -s standard -s "prefix(github.com/harness/)" -s default -s blank -s dot .
	@echo "Formatting complete"
