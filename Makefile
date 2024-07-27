server:
	go build -ldflags "-X main.version=$$(git describe --exact-match --tags || echo "dev" ) -X main.commit=$$(git rev-parse --short HEAD)" -o mobius-hotline-server cmd/mobius-hotline-server/main.go
