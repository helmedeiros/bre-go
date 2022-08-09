FROM golang:1.22-alpine

RUN apk add --no-cache bash diffutils coreutils

WORKDIR /work

COPY go.mod go.sum* ./
COPY engine/ engine/
COPY observability/ observability/
COPY scientific/v0.15.0/go.mod scientific/v0.15.0/go.sum* scientific/v0.15.0/
COPY scientific/v0.15.0/schema/ scientific/v0.15.0/schema/
COPY scientific/v0.15.0/cmd/ scientific/v0.15.0/cmd/

WORKDIR /work/scientific/v0.15.0

RUN go mod download && \
    go build -trimpath -o /usr/local/bin/scigen      ./cmd/scigen      && \
    go build -trimpath -o /usr/local/bin/sci-build   ./cmd/build       && \
    go build -trimpath -o /usr/local/bin/sci-load    ./cmd/load        && \
    go build -trimpath -o /usr/local/bin/sci-compare ./cmd/compare     && \
    go build -trimpath -o /usr/local/bin/sci-stats   ./cmd/stats       && \
    go build -trimpath -o /usr/local/bin/sci-adv     ./cmd/adversarial && \
    go build -trimpath -o /usr/local/bin/sci-refusals ./cmd/refusals

WORKDIR /data

ENTRYPOINT ["/bin/bash"]
