FROM golang:1.22-alpine

RUN apk add --no-cache bash diffutils coreutils

WORKDIR /work

COPY go.mod go.sum* ./
COPY engine/ engine/
COPY observability/ observability/

# v0.15.0 baseline harness (scigen, sci-build, sci-load, sci-compare, sci-stats)
COPY scientific/v0.15.0/go.mod scientific/v0.15.0/
COPY scientific/v0.15.0/schema/ scientific/v0.15.0/schema/
COPY scientific/v0.15.0/cmd/ scientific/v0.15.0/cmd/

# Experimental harness (C1, C3, C4, C5)
COPY scientific/v0.15.0/experimental/ scientific/v0.15.0/experimental/

# Build v0.15.0 baseline binaries
WORKDIR /work/scientific/v0.15.0
RUN go mod download && \
    go build -trimpath -o /usr/local/bin/scigen      ./cmd/scigen      && \
    go build -trimpath -o /usr/local/bin/sci-build   ./cmd/build       && \
    go build -trimpath -o /usr/local/bin/sci-load    ./cmd/load        && \
    go build -trimpath -o /usr/local/bin/sci-compare ./cmd/compare     && \
    go build -trimpath -o /usr/local/bin/sci-stats   ./cmd/stats

# Build experimental binaries
WORKDIR /work/scientific/v0.15.0/experimental
RUN go mod download && \
    go build -trimpath -o /usr/local/bin/sci-build-c1 ./cmd/build-c1 && \
    go build -trimpath -o /usr/local/bin/sci-load-c1  ./cmd/load-c1  && \
    go build -trimpath -o /usr/local/bin/sci-build-c3 ./cmd/build-c3 && \
    go build -trimpath -o /usr/local/bin/sci-load-c3  ./cmd/load-c3  && \
    go build -trimpath -o /usr/local/bin/sci-build-c4 ./cmd/build-c4 && \
    go build -trimpath -o /usr/local/bin/sci-load-c4  ./cmd/load-c4  && \
    go build -trimpath -o /usr/local/bin/sci-build-c5 ./cmd/build-c5 && \
    go build -trimpath -o /usr/local/bin/sci-load-c5  ./cmd/load-c5

WORKDIR /data
ENTRYPOINT ["/bin/bash"]
