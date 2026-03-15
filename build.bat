
cd %~dp0%
go clean
go build -ldflags="-H=windowsgui" -o localsend-go.exe .
