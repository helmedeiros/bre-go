#!/usr/bin/env bash
# Extract the first ```go ... ``` block from README.md, build and run it as
# an external module that imports bre-go via a replace directive, and verify
# the stdout matches the // Output: comment in the block. Catches drift
# between the README example and the actual public API.
set -euo pipefail

README="README.md"
if [ ! -f "$README" ]; then
  echo "missing $README" >&2
  exit 1
fi

workdir="$(mktemp -d -t bre-go-quickstart.XXXXXX)"
trap 'rm -rf "$workdir"' EXIT

cat > "$workdir/go.mod" <<EOF
module quickstart

go 1.18

require github.com/helmedeiros/bre-go v0.0.0

replace github.com/helmedeiros/bre-go => $PWD
EOF

awk '/^```go$/{flag=1; next} /^```$/{if(flag){flag=0; exit}} flag' "$README" > "$workdir/main.go"

if [ ! -s "$workdir/main.go" ]; then
  echo "no go code block found in $README" >&2
  exit 1
fi

expected=$(awk '/\/\/ Output:/{
  sub(/^[[:space:]]*\/\/ Output:[[:space:]]*/, "");
  print;
  flag=1;
  next
} flag && /^[[:space:]]*\/\/ /{
  sub(/^[[:space:]]*\/\/ */, "");
  print
} flag && !/^[[:space:]]*\/\/ /{
  exit
}' "$workdir/main.go")

if [ -z "$expected" ]; then
  echo "no // Output: comment found in quickstart" >&2
  exit 1
fi

actual=$(cd "$workdir" && go run main.go 2>&1)

if [ "$actual" != "$expected" ]; then
  echo "Quickstart output mismatch:" >&2
  echo "  expected: $expected" >&2
  echo "  actual:   $actual" >&2
  exit 1
fi

echo "Quickstart in sync ($(wc -l < "$workdir/main.go" | tr -d ' ') lines, expected output matched)"
