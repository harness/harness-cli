.PHONY: all build clean generate test deps setup-tools

# Default target
all: deps generate build

# Build the CLI
build:
	mkdir -p bin
	go build -o bin/hns ./cmd/hns

# Clean build artifacts
clean:
	rm -rf bin/
	find ./pkg/services -name "generated" -type d -exec rm -rf {} +;

# Generate code from OpenAPI specs
generate: setup-tools
	@echo "Generating code from OpenAPI specs..."
	@for spec in api-specs/*.yaml; do \
		service=$$(basename $$spec .yaml); \
		echo "Processing $$service service..."; \
		mkdir -p ./pkg/services/$$service/generated; \
		go run ./tools/generator/main.go -spec=$$spec -service=$$service -output=./pkg/services/$$service/generated; \
	done

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod tidy
	go get github.com/go-resty/resty/v2
	go get github.com/olekukonko/tablewriter
	go get github.com/spf13/cobra
	go get github.com/getkin/kin-openapi
	@if ! command -v oapi-codegen > /dev/null; then \
		echo "Installing oapi-codegen..."; \
		go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest; \
	fi

# Setup tools directory
setup-tools:
	@mkdir -p tools/generator
