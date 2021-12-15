package proxy

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Connection struct {
	IsMultiplexStream bool
}

// Proxy - Manages a Proxy connection, piping data between local and remote.
type Proxy struct {
	sentBytes     uint64
	receivedBytes uint64
	laddr, raddr  *net.TCPAddr
	lconn, rconn  io.ReadWriteCloser
	erred         bool
	errsig        chan bool
	tlsUnwrapp    bool
	tlsAddress    string

	Matcher  func([]byte)
	Replacer func([]byte) []byte

	// Settings
	Nagles    bool
	Log       Logger
	OutputHex bool
}

// New - Create a new Proxy instance. Takes over local connection passed in,
// and closes it when finished.
func New(lconn *net.TCPConn, laddr, raddr *net.TCPAddr) *Proxy {
	return &Proxy{
		lconn:  lconn,
		laddr:  laddr,
		raddr:  raddr,
		erred:  false,
		errsig: make(chan bool),
		Log:    NullLogger{},
	}
}

// NewTLSUnwrapped - Create a new Proxy instance with a remote TLS server for
// which we want to unwrap the TLS to be able to connect without encryption
// locally
func NewTLSUnwrapped(lconn *net.TCPConn, laddr, raddr *net.TCPAddr, addr string) *Proxy {
	p := New(lconn, laddr, raddr)
	p.tlsUnwrapp = true
	p.tlsAddress = addr
	return p
}

type setNoDelayer interface {
	SetNoDelay(bool) error
}

// Start - open connection to remote and start proxying data.
func (p *Proxy) Start() {
	defer p.lconn.Close()

	var err error
	//connect to remote
	if p.tlsUnwrapp {
		p.rconn, err = tls.Dial("tcp", p.tlsAddress, nil)
	} else {
		p.rconn, err = net.DialTCP("tcp", nil, p.raddr)
	}
	if err != nil {
		p.Log.Warn("Remote connection failed: %s", err)
		return
	}
	defer p.rconn.Close()

	//nagles?
	if p.Nagles {
		if conn, ok := p.lconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
		if conn, ok := p.rconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
	}

	//display both ends
	p.Log.Info("Opened %s to %s", p.laddr.String(), p.raddr.String())

	conn := Connection{}

	//bidirectional copy
	go p.pipe(p.lconn, p.rconn, &conn)
	go p.pipe(p.rconn, p.lconn, &conn)

	//wait for close...
	<-p.errsig
	p.Log.Info("Closed %s to %s (%d bytes sent, %d bytes recieved)", p.laddr.String(), p.raddr.String(), p.sentBytes, p.receivedBytes)
}

func (p *Proxy) err(s string, err error, waitBothToClose bool) {

	if err != io.EOF {
		p.Log.Warn(s, err)
	} else {
		p.Log.Info(s, err)
	}

	if p.erred || !waitBothToClose {
		p.errsig <- true
	}

	p.erred = true
}

func (p *Proxy) pipe(src, dst io.ReadWriter, conn *Connection) {

	isSending := src == p.lconn

	var dataDirection string
	var header string
	var prefix string

	if isSending {
		dataDirection = ">>> %d bytes sent%s"
		header = ">>> Sending:"
		prefix = ">>> "
	} else {
		dataDirection = "<<< %d bytes received%s"
		header = "<<< Receiving:"
		prefix = "<<< "
	}

	p.Log.Trace(prefix + "Pipe started")

	var byteFormat string
	if p.OutputHex {
		byteFormat = "%x"
	} else {
		byteFormat = "%s"
	}

	//directional copy
	buff := make([]byte, 0xfffff)
	for {

		var bu bytes.Buffer

		eof := false
		first := true
		for first || (!eof && !conn.IsMultiplexStream) {
			n, err := src.Read(buff)
			if first {
				p.Log.Trace(header)
				first = false
			}
			p.Log.Trace(prefix+"buffer read: %d", n)
			if err != nil {
				p.err(prefix+"Read failed '%s'\n", err, conn.IsMultiplexStream && isSending && err == io.EOF) //in case it's streaming & sending data & EOF, we wait for the receiving EOF to stop the connection
				return
			}
			eof = n < 1 || buff[n-1] == 10
			bu.Write(buff[:n])
		}

		n := bu.Len()
		b := bu.Bytes()

		//execute match
		if p.Matcher != nil {
			p.Matcher(b)
		}

		//execute replace
		if p.Replacer != nil {
			b = p.Replacer(b)
		}

		if !conn.IsMultiplexStream {
			if isSending {
				//bytes sent
				//Replace windows drive into linux mount foe ex C:\test -> /mnt/c/test

				re, _ := regexp.Compile("[\", ][A-Za-z]\\:((\\\\\\\\|\\/)[\\w\\-. ]+)+(\\\\\\\\|\\/)?[^\\\\\\/\\w\\-. ]")
				b = []byte(p.editHttpMessage(string(b), func(w string) string {
					ret := re.ReplaceAllStringFunc(w, func(s string) string {
						return string(s[0]) + "/mnt/" + string(unicode.ToLower(rune(s[1]))) + strings.ReplaceAll(s[3:], "\\\\", "/")
					})
					return ret
				}))

			} else {

				//bytes received
				re, _ := regexp.Compile("\\/mnt\\/[a-z](\\/[\\w\\-. ]+)+[^\\\\\\/\\w\\-. ]")

				b = []byte(p.editHttpMessage(string(b), func(w string) string {
					ret := re.ReplaceAllStringFunc(w, func(w string) string {
						s := string(w)
						return string(unicode.ToUpper(rune(s[5]))) + ":" + strings.ReplaceAll(s[6:], "/", "\\\\")
					})
					return ret
				}))

				if strings.Contains(strings.ToLower(getHttpHeader(string(b))), strings.ToLower("Content-Type: application/vnd.docker.raw-stream")) {
					p.Log.Debug("MultiplexStream: true")
					conn.IsMultiplexStream = true
				}

			}
		}

		//show output
		p.Log.Debug(dataDirection, n, "")
		p.Log.Trace(byteFormat, b)

		//write out result
		n, err := dst.Write(b)
		if err != nil {
			p.err("Write failed '%s'\n", err, conn.IsMultiplexStream)
			return
		}
		if isSending {
			p.sentBytes += uint64(n)
		} else {
			p.receivedBytes += uint64(n)
		}
	}
}

func (p *Proxy) editHttpMessage(httpMessage string, edit func(payload string) string) string {
	//find payload
	start := getHttpPayloadStart(httpMessage)
	if start < 0 {
		return httpMessage
	}
	payload := httpMessage[start:]
	//convert

	header := httpMessage[0:start]
	isChunked := strings.Contains(strings.ToLower(header), strings.ToLower("Transfer-Encoding: chunked"))

	var editedHttpMessage string
	if isChunked {
		//chunked
		newPayload := p.editChunkedPayload(payload, edit)
		editedHttpMessage = header + newPayload
	} else {
		//not chunked
		newPayload := edit(payload)
		editedHttpMessage = header + newPayload
		editedHttpMessage = p.fixContentLength(httpMessage, editedHttpMessage)
	}

	return editedHttpMessage
}

func getHttpHeader(httpMessage string) string {
	start := getHttpPayloadStart(httpMessage)
	if start < 0 {
		return httpMessage
	}
	return httpMessage[0:start]
}

func getHttpPayloadStart(httpMessage string) int {
	start := strings.Index(httpMessage, "\r\n\r\n")
	if start < 0 {
		return start
	}
	start += 4
	return start
}

func (p *Proxy) editChunkedPayload(payload string, edit func(payload string) string) string {

	ret := ""

	for {
		//find chunk size
		index := strings.Index(payload, "\r\n")
		if index < 0 {
			return ret
		}
		payloadStart := index + 2
		chunkSizeS := payload[0 : payloadStart-2]
		chunkSize, _ := strconv.ParseInt(chunkSizeS, 16, 64)

		if chunkSize == 0 {
			ret += payload
			return ret
		}

		chunkPayload := payload[payloadStart : payloadStart+int(chunkSize)]

		//debug
		//currentPayloadSize := strconv.FormatInt(int64(len(chunkPayload)), 16)
		//currentPayloadSize = currentPayloadSize + ""

		editChunkPayload := edit(chunkPayload)
		ret += strconv.FormatInt(int64(len(editChunkPayload)), 16) + "\r\n" + editChunkPayload + "\r\n"
		start := payloadStart + int(chunkSize) + 2
		if start >= len(payload) {
			payload = ""
		} else {
			payload = payload[start:]
		}
	}

}

func (p *Proxy) fixContentLength(originalData string, newData string) string {
	reLe, _ := regexp.Compile("Content-Length: \\d*")
	found := false
	e := reLe.ReplaceAllStringFunc(newData, func(w string) string {
		if !found {
			s := string(w)
			n := s[strings.LastIndex(s, ":")+2:]
			numberOfCharacters, err := strconv.Atoi(n)
			if err != nil {
				// handle error
				p.Log.Warn(err.Error())
			}
			numberOfCharacters += len(newData) - len(originalData)
			found = true
			return "Content-Length: " + strconv.Itoa(numberOfCharacters)
		} else {
			return w
		}
	})

	return e
}
