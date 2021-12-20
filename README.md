# tcp-proxy

A small TCP proxy written in Go

This project was intended for debugging text-based protocols. The next version will address binary protocols.

Compatible with Docker Engine API 1.41. https://docs.docker.com/engine/api/v1.41/
Especially with protocol switching (HTTP code 101):
- interactive session that "hijacks the HTTP connection to HTTP2 transport"
- container attached where "endpoint hijacks the HTTP connection to transport stdin, stdout, and stderr on the same socket".


## Install

**Binaries**

Download [the latest release](https://github.com/jpillora/go-tcp-proxy/releases/latest), or

Install latest release now with `curl https://i.jpillora.com/go-tcp-proxy! | bash`

$env:GOARCH="amd64"
$env:GOOS="windows"
go build -o ./bin/proxy-docker.exe .\cmd\tcp-proxy\main.go

$env:GOARCH="amd64"
$env:GOOS="linux"
go build -o ./bin/proxy-docker .\cmd\tcp-proxy\main.go


**Source**

``` sh
$ go get -v github.com/jpillora/go-tcp-proxy/cmd/tcp-proxy
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
  -match="": match regex (in the form 'regex')
  -replace="": replace regex (in the form 'regex~replacer')
  -v: display server actions
  -vv: display server actions and all tcp data
```

*Note: Regex match and replace*
**only works on text strings**
*and does NOT work across packet boundaries*

### Simple Example

Since HTTP runs over TCP, we can also use `tcp-proxy` as a primitive HTTP proxy:

```
$ tcp-proxy -r echo.jpillora.com:80
Proxying from localhost:9999 to echo.jpillora.com:80
```

Then test with `curl`:

```
$ curl -H 'Host: echo.jpillora.com' localhost:9999/foo
{
  "method": "GET",
  "url": "/foo"
  ...
}
```

### Match Example

```
$ tcp-proxy -r echo.jpillora.com:80 -match 'Host: (.+)'
Proxying from localhost:9999 to echo.jpillora.com:80
Matching Host: (.+)

#run curl again...

Connection #001 Match #1: Host: echo.jpillora.com
```

### Replace Example

```
$ tcp-proxy -r echo.jpillora.com:80 -replace '"ip": "([^"]+)"~"ip": "REDACTED"'
Proxying from localhost:9999 to echo.jpillora.com:80
Replacing "ip": "([^"]+)" with "ip": "REDACTED"
```

```
#run curl again...
{
  "ip": "REDACTED",
  ...
```

*Note: The `-replace` option is in the form `regex~replacer`. Where `replacer` may contain `$N` to substitute in group `N`.*

### Todo

* Implement `tcpproxy.Conn` which provides accounting and hooks into the underlying `net.Conn`
* Verify wire protocols by providing `encoding.BinaryUnmarshaler` to a `tcpproxy.Conn`
* Modify wire protocols by also providing a map function
* Implement [SOCKS v5](https://www.ietf.org/rfc/rfc1928.txt) to allow for user-decided remote addresses