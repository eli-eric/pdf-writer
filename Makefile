.PHONY: help check test race cover fmt vet lint generate example clean

# AFM points at a directory of Adobe AFM metric files, needed only by
# `make generate`. The URW base35 set that ships with Ghostscript works.
AFM ?=

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk -F':.*?## ' '{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

check: fmt vet lint race ## Run everything CI runs

test: ## Run the test suite
	go test ./...

race: ## Run the test suite with the race detector
	go test -race ./...

cover: ## Report test coverage
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	@echo "for a browsable report: go tool cover -html=coverage.out"

fmt: ## Check formatting
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	go vet ./...

lint: ## Run staticcheck if it is installed
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed, skipping"; \
		echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"; \
	fi

generate: ## Regenerate stdfont_metrics.go (needs AFM=/path/to/afm/dir)
	@if [ -z "$(AFM)" ]; then \
		echo "usage: make generate AFM=/path/to/afm/dir"; exit 1; \
	fi
	go run ./internal/gen -afm "$(AFM)" > stdfont_metrics.go
	gofmt -w stdfont_metrics.go

example: ## Build the example report
	go run ./examples/report

clean: ## Remove generated artefacts
	rm -f coverage.out report.pdf czech.pdf
