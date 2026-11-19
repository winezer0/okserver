set CGO_ENABLED=0
set ldflags="-buildid=janbar -s -w"
set GOARCH=amd64

set GOOS=linux
go build -ldflags %ldflags% -trimpath -o okserver-linux-amd64 ./cmd/okserver

set GOOS=windows
go build -ldflags %ldflags% -trimpath -o okserver-windows-amd64.exe ./cmd/okserver

where /q upx && upx -9 okserver-linux-amd64 okserver-windows-amd64.exe
