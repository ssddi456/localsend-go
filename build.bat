
cd %~dp0%
go clean
go build -o localsend-go.exe .
localsend-go.exe receive
