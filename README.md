# windows2wsl-docker-proxy

A TCP proxy written in Go to adapt windows docker clients to a linux docker running in WSL.

[Original generic TCP proxy](https://github.com/jpillora/go-tcp-proxy)

Compatible with Docker Engine API 1.41. https://docs.docker.com/engine/api/v1.41/
Especially with protocol switching (HTTP code 101):
- interactive session that "hijacks the HTTP connection to HTTP2 transport"
- container attached where "endpoint hijacks the HTTP connection to transport stdin, stdout, and stderr on the same socket".


## Install

**Binaries**

Download [the latest release](https://github.com/alexvaut/windows2wsl-docker-proxy/releases/latest)


$env:GOARCH="amd64"
$env:GOOS="windows"
go build -o ./bin/proxy-docker.exe .\cmd\tcp-proxy\main.go

$env:GOARCH="amd64"
$env:GOOS="linux"
go build -o ./bin/proxy-docker .\cmd\tcp-proxy\main.go


**Source**

``` sh
$ go get -v github.com/alexvaut/windows2wsl-docker-proxy/cmd/tcp-proxy
```

## Usage

```
$ tcp-proxy --help
Usage of tcp-proxy:
  -c: output ansi colors
  -h: output hex
  -l="localhost:9999": local address
  -n: disable nagles algorithm
  -r="localhost:80": remote address  
  -v: display server actions
  -vv: display server actions and all tcp data
```
### Simple Example

Opens port 2376 and redirect to docker running on 2375

```
$ tcp-proxy -r localhost:2375 -l :2376
Proxying from :2376 to localhost:2375
```
