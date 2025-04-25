# Makefile for generating REST clients + Cobra commands from all OpenAPI specs

API_DIR := api
SERVICES := $(shell find $(API_DIR) -mindepth 2 -maxdepth 2 -type f -name "openapi.yaml" | \
	            sed -E "s|$(API_DIR)/([^/]+)/openapi.yaml|\1|")

GOCMD := go
GEN   := ./tools/cobra-gen

.PHONY: generate build clean

# Generate code for every service folder that owns an openapi.yaml
generate: $(SERVICES:%=generate-%)

generate-%:
	@echo ">> Generating $*"
	$(GOCMD) run $(GEN) --service=$* --in=$(API_DIR)/$*/openapi.yaml --out=cmd/$*

# Build the CLI (runs `generate` first so everything is up to date)
build: generate
	$(GOCMD) build ./...

# Remove generated artifacts
clean:
	rm -rf internal/api/*
	rm -f  cmd/*/*_gen.go
	rm -f  cmd/*/gen/*.go
