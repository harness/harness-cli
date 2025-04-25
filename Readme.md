Harness CLI V2 design
1. We use openapi.yaml files for each service to build internal cobra commands and their types + clients using oapi-codegen.
   1. Command: `make generate`
2. Cobra commands are not that helpful at this moment. But to overcome this, we can write them manually.
3. To build, use `go build -o hns ./cmd/`
4. To override or additional commands, add additional handlers in `cmd/<service>`
5. The template resides in `tools/cobra-gen`. Basic overrides can be done there.  