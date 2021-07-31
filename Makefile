linux_client_arm_target=dist/mobius_client_linux_arm
build-linux-arm-client:
	mkdir $(linux_client_arm_target) ; true
	GOOS=linux GOARCH=arm go build -o $(linux_client_arm_target)/mobius-hotline-client  cmd/mobius-hotline-client/main.go

package-linux-arm-client: build-linux-arm-client
	cp cmd/mobius-hotline-client/mobius-client-config.yaml $(linux_client_arm_target)
	tar -zcvf $(linux_client_arm_target).tar.gz $(linux_client_arm_target)

linux_client_amd64_target=dist/mobius_client_linux_amd64
build-linux-amd64-client:
	mkdir $(linux_client_amd64_target) ; true
	GOOS=linux GOARCH=amd64 go build -o $(linux_client_amd64_target)/mobius-hotline-client  cmd/mobius-hotline-client/main.go

package-linux-amd64-client: build-linux-amd64-client
	cp cmd/mobius-hotline-client/mobius-client-config.yaml $(linux_client_amd64_target)
	tar -zcvf $(linux_client_amd64_target).tar.gz $(linux_client_amd64_target)


linux_server_arm_target=dist/mobius_server_linux_arm
build-linux-arm-server:
	mkdir $(linux_server_arm_target) ; true
	GOOS=linux GOARCH=arm go build -o $(linux_server_arm_target)/mobius-hotline-server  cmd/mobius-hotline-server/main.go

package-linux-arm-server: build-linux-arm-server
	cp -r cmd/mobius-hotline-server/mobius/config $(linux_server_arm_target)
	tar -zcvf $(linux_server_arm_target).tar.gz $(linux_server_arm_target)

linux_server_amd64_target=dist/mobius_server_linux_amd64
build-linux-amd64-server:
	mkdir $(linux_server_amd64_target) ; true
	GOOS=linux GOARCH=amd64 go build -o $(linux_server_amd64_target)/mobius-hotline-server  cmd/mobius-hotline-server/main.go

package-linux-amd64-server: build-linux-amd64-server
	cp -r cmd/mobius-hotline-server/mobius/config $(linux_server_amd64_target)
	tar -zcvf $(linux_server_amd64_target).tar.gz $(linux_server_amd64_target)

darwin_server_amd64_target=dist/mobius_server_darwin_amd64
build-darwin-amd64-server:
	mkdir $(darwin_server_amd64_target) ; true
	GOOS=darwin GOARCH=amd64 go build -o $(darwin_server_amd64_target)/mobius-hotline-server  cmd/mobius-hotline-server/main.go

package-darwin-amd64-server: build-darwin-amd64-server
	cp -r cmd/mobius-hotline-server/mobius/config $(darwin_server_amd64_target)
	tar -zcvf dist/mobius_server_darwin_amd64.tar.gz $(darwin_server_amd64_target)

darwin_client_amd64_target=dist/mobius_client_darwin_amd64
build-darwin-amd64-client:
	mkdir $(darwin_client_amd64_target) ; true
	GOOS=darwin GOARCH=amd64 go build -o $(darwin_client_amd64_target)/mobius-hotline-client  cmd/mobius-hotline-client/main.go

package-darwin-amd64-client: build-darwin-amd64-client
	cp cmd/mobius-hotline-client/mobius-client-config.yaml $(darwin_client_amd64_target)
	tar -zcvf dist/mobius_client_darwin_amd64.tar.gz $(darwin_client_amd64_target)

windows_client_amd64_target=dist/mobius_client_windows_amd64
build-win-amd64-client:
	mkdir $(windows_client_amd64_target) ; true
	GOOS=windows GOARCH=amd64 go build -o $(windows_client_amd64_target)/mobius-hotline-client.exe  cmd/mobius-hotline-client/main.go

package-win-amd64-client: build-win-amd64-client
	cp cmd/mobius-hotline-client/mobius-client-config.yaml $(windows_client_amd64_target)
	zip -r dist/mobius_client_windows_amd64.zip $(windows_client_amd64_target)

windows_server_amd64_target=dist/mobius_server_windows_amd64
build-win-server-amd64:
	mkdir $(windows_server_amd64_target) ; true
	GOOS=windows GOARCH=amd64 go build -o $(windows_server_amd64_target)/mobius-hotline-server.exe  cmd/mobius-hotline-server/main.go

package-win-amd64-server: build-win-server-amd64
	cp -r cmd/mobius-hotline-server/mobius/config $(windows_server_amd64_target)
	zip -r dist/mobius_server_windows_amd64.zip $(windows_server_amd64_target)

all: clean \
	package-win-amd64-server \
	package-win-amd64-client \
 	package-darwin-amd64-client \
 	package-darwin-amd64-server \
 	package-linux-arm-server \
 	package-linux-amd64-server \
 	package-linux-arm-client \
 	package-linux-amd64-client \

clean:
	rm -rf dist/*