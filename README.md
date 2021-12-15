# windows2wsl-docker-proxy

A TCP proxy written in Go to adapt windows docker clients to a linux docker running in WSL.

[Original generic TCP proxy](https://github.com/jpillora/go-tcp-proxy)

## Install

**Binaries**

Download [the latest release](https://github.com/alexvaut/windows2wsl-docker-proxy/releases/latest)


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
