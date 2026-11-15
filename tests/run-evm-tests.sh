#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

git submodule update --init --depth 1 --recursive tests/testdata
if [[ ! -d tests/spec-tests/fixtures ]]; then
  mkdir -p tests/spec-tests
  curl --fail --location --retry 3 \
    https://github.com/ethereum/execution-spec-tests/releases/download/pectra-devnet-6%40v1.0.0/fixtures_pectra-devnet-6.tar.gz \
    | tar xz -C tests/spec-tests
fi

# Core-only precompiles and opcodes are selected by Satoshi rules; Ethereum
# fixtures no longer need a patch to the production VM implementation.
if ! go test -p=1 -parallel=2 -timeout=45m -short -v ./tests > tests/test.log 2>&1; then
  tail -n 100 tests/test.log
  exit 1
fi
tail -n 1 tests/test.log
