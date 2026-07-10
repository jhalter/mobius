server:
	go build -ldflags "-X main.version=$$(git describe --exact-match --tags || echo "dev" ) -X main.commit=$$(git rev-parse --short HEAD)" -o mobius-hotline-server cmd/mobius-hotline-server/main.go

.PHONY: test
test:
	go test ./... -race -shuffle=on

.PHONY: cover
cover:
	go test ./... -race -shuffle=on -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out

.PHONY: lint
lint:
	golangci-lint run

# Run each fuzz target for FUZZTIME (default 30s). Plain `go test` already replays
# committed seed corpora; this exercises new random inputs.
.PHONY: fuzz
fuzz:
	@for target in $$(go test ./hotline -list 'Fuzz.*' | grep '^Fuzz'); do \
		echo "fuzzing $$target"; \
		go test ./hotline -run '^$$' -fuzz "^$$target$$" -fuzztime $${FUZZTIME:-30s} || exit 1; \
	done
