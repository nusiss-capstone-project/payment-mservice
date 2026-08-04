package proxy

// Generate proxy mocks with mockery v2.
//
// Install (once):
//   go install github.com/vektra/mockery/v2@v2.53.5
//
// Generate:
//   go generate ./proxy/...
// or from repo root:
//   bash scripts/generate_mocks.sh
//
//go:generate mockery --output=./mocks --outpkg=mocks --all
