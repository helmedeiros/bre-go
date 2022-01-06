SHELL := /bin/bash

GO        ?= go
PKG       := ./...
COVER_OUT ?= coverage.out
COVER_MIN ?= 80

.PHONY: help tools lint vet test cover cover-html all ci-local clean

help:
	@echo "Targets:"
	@echo "  tools       - install development tools (golangci-lint)"
	@echo "  lint        - run golangci-lint"
	@echo "  vet         - run go vet"
	@echo "  test        - run go test with the race detector"
	@echo "  cover       - run tests with coverage and enforce \$$COVER_MIN ($(COVER_MIN)%)"
	@echo "  cover-html  - open the per-line HTML coverage report"
	@echo "  all         - lint + vet + test + cover"
	@echo "  ci-local    - the same checks CI runs, in the same order"
	@echo "  clean       - remove generated coverage artifacts"

tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2

lint:
	golangci-lint run

vet:
	$(GO) vet $(PKG)

test:
	$(GO) test -race -count=1 $(PKG)

cover:
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=$(COVER_OUT) $(PKG)
	@total=$$($(GO) tool cover -func=$(COVER_OUT) | awk '/^total:/{print $$3}' | tr -d '%'); \
	echo "coverage: $$total%"; \
	awk -v have="$$total" -v need="$(COVER_MIN)" 'BEGIN{ exit (have+0 >= need+0) ? 0 : 1 }' \
	  || { echo "coverage below threshold ($(COVER_MIN)%)"; exit 1; }

cover-html: cover
	$(GO) tool cover -html=$(COVER_OUT)

all: lint vet test cover

ci-local: all

clean:
	rm -f $(COVER_OUT)
