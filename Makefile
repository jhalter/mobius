build-client:
	go build -o mobius-hotline-client client/main.go

build-server:
	go build -o mobius-hotline-server server/main.go

windows_amd64_target=dist/mobius_server_windows_amd64
build-win-amd64-server:
	mkdir $(windows_amd64_target) ; true
	cp -r cmd/mobius-hotline-server/mobius/config $(windows_amd64_target)
	GOOS=windows GOARCH=amd64 go build -o $(windows_amd64_target)/mobius-hotline-server.exe  cmd/mobius-hotline-server/main.go
	zip -r dist/mobius_server_windows_amd64.zip $(windows_amd64_target)