GO ?= go

.PHONY: fmt vet lint test race coverage build tidy dev-test leaklab

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

coverage:
	GO=$(GO) ./scripts/check-coverage.sh

build:
	$(GO) build -trimpath ./cmd/agentfence

tidy:
	$(GO) mod tidy

dev-test:
	./scripts/dev-test.sh

leaklab:
	$(GO) test -run TestLeakLab ./...
