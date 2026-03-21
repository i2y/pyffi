.PHONY: all build test lint vet fmt bench ci clean

all: ci

# Build all packages
build:
	go build ./...

# Run tests
test:
	go test -count=1 -v ./...

# Run tests (short, no verbose)
test-short:
	go test -count=1 ./...

# Check formatting (fails if any file needs formatting)
fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

# Run go vet (disable unsafeptr check for purego C-pointer conversions)
vet:
	go vet -unsafeptr=false ./...

# Run staticcheck if available
lint:
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping (go install honnef.co/go/tools/cmd/staticcheck@latest)"

# Run benchmarks
bench:
	go test -bench=. -benchtime=200ms -count=1 -run='^$$' -benchmem .

# Full CI pipeline: fmt + vet + build + test
ci: fmt vet build test-short
	@echo "CI passed."

# Clean build cache
clean:
	go clean -cache -testcache
