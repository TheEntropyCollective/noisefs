#!/bin/bash
# Fix remaining testing references
sed -i.bak 's/testing\./fixtures\./g' cmd/noisefs-tools/benchmark/docker-benchmark/main.go
sed -i.bak 's/\*testing\./\*fixtures\./g' cmd/noisefs-tools/benchmark/docker-benchmark/main.go
sed -i.bak 's/ testing\./ fixtures\./g' cmd/noisefs-tools/benchmark/docker-benchmark/main.go