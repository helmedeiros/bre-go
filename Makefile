SHELL := /bin/bash

GO        ?= go
PKG       := ./...
COVER_OUT ?= coverage.out
COVER_MIN ?= 80

.PHONY: help tools lint vet test cover cover-html bench all ci-local clean

help:
	@echo "Targets:"
	@echo "  tools       - install development tools (golangci-lint)"
	@echo "  lint        - run golangci-lint"
	@echo "  vet         - run go vet"
	@echo "  test        - run go test with the race detector"
	@echo "  cover       - run tests with coverage and enforce \$$COVER_MIN ($(COVER_MIN)%)"
	@echo "  cover-html  - open the per-line HTML coverage report"
	@echo "  bench       - run benchmarks across all packages"
	@echo "  all         - lint + vet + test + cover"
	@echo "  ci-local    - the same checks CI runs, in the same order"
	@echo "  clean       - remove generated coverage artifacts"

tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2

# Each gate vacuously passes when the module has no Go files yet.
# As soon as the first *.go lands, the guards drop through to the
# real tool invocation. This lets the gates be on from commit one.
HAS_GO := $(shell find . -name '*.go' -not -path './.git/*' -print -quit)

lint:
ifneq ($(HAS_GO),)
	golangci-lint run
else
	@echo "no go files to lint -- skipping"
endif

vet:
ifneq ($(HAS_GO),)
	$(GO) vet $(PKG)
else
	@echo "no go files to vet -- skipping"
endif

test:
ifneq ($(HAS_GO),)
	$(GO) test -race -count=1 $(PKG)
else
	@echo "no go files to test -- skipping"
endif

cover:
ifneq ($(HAS_GO),)
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=$(COVER_OUT) $(PKG)
	@# Drop test-only helper packages (anything ending in "test")
	@# from the coverage profile before checking the threshold; these
	@# packages exist to assist real tests and have no production code.
	@grep -v '/enginetest/' $(COVER_OUT) > $(COVER_OUT).prod || true
	@mv $(COVER_OUT).prod $(COVER_OUT)
	@# A coverage.out with only the "mode:" header means no statements
	@# were measured anywhere -- threshold is vacuously satisfied.
	@if [ "$$(wc -l < $(COVER_OUT) | tr -d ' ')" -le 1 ]; then \
	  echo "coverage: no executable statements -- vacuously pass"; \
	  exit 0; \
	fi; \
	total=$$($(GO) tool cover -func=$(COVER_OUT) | awk '/^total:/{print $$3}' | tr -d '%'); \
	echo "coverage: $$total%"; \
	awk -v have="$$total" -v need="$(COVER_MIN)" 'BEGIN{ exit (have+0 >= need+0) ? 0 : 1 }' \
	  || { echo "coverage below threshold ($(COVER_MIN)%)"; exit 1; }
else
	@echo "no go files to measure -- skipping"
endif

cover-html: cover
	$(GO) tool cover -html=$(COVER_OUT)

bench:
ifneq ($(HAS_GO),)
	$(GO) test -run=^$$ -bench=. -benchmem $(PKG)
else
	@echo "no go files to benchmark -- skipping"
endif

all: lint vet test cover

ci-local: all

clean:
	rm -f $(COVER_OUT)
