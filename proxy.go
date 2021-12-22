package proxy

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"regexp"
	"strings"
	"unicode"
)

type Connection struct {
	Same bool //true if the payload must remain the same between client and server.
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
		dataDirection = ">>> ## => Total of %d bytes sent%s"
		header = ">>> ## Sending:"
		prefix = ">>> "
	} else {
		dataDirection = "<<< ## => Total of %d bytes received%s"
		header = "<<< ## Receiving:"
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
	chunked := false
	buff := make([]byte, 0xfffff)
	for {

		var bu bytes.Buffer

		eof := false
		first := true

		for !eof {
			n, err := src.Read(buff)
			if first {
				p.Log.Trace(header)
				first = false
			}
			p.Log.Trace(prefix+"   Buffer read: %d", n)
			p.Log.Trace(byteFormat, buff[:n])
			if err != nil {
				p.err(prefix+"Read failed '%s'\n", err, isSending && err == io.EOF) //in case it's streaming & sending data & EOF, we wait for the receiving EOF to stop the connection
				return
			}

			bu.Write(buff[:n])
			complete := false

			if !conn.Same {
				if !chunked {
					complete, chunked = isComplete(string(bu.Bytes()), isSending)
					if chunked {
						p.Log.Trace("   Start of Chunk stream detected")
					}
				}

				if chunked {
					lastChunkSent := false
					complete, lastChunkSent = isLastChunkComplete(string(bu.Bytes()))
					if lastChunkSent {
						chunked = false
						p.Log.Trace("   End of Chunk stream detected")
					}
				}
			}

			eof = conn.Same || complete
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

		if !conn.Same {
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

				if strings.Contains(strings.ToLower(getHttpHeader(string(b))), strings.ToLower("Upgrade: h2c")) {
					p.Log.Debug("MultiplexStream: true")
					conn.Same = true
				}

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
					conn.Same = true
				}

			}
		}

		//show output
		p.Log.Debug(dataDirection, n, "")

		//write out result
		n, err := dst.Write(b)
		if err != nil {
			p.err("Write failed '%s'\n", err, conn.Same)
			return
		}
		if isSending {
			p.sentBytes += uint64(n)
		} else {
			p.receivedBytes += uint64(n)
		}
	}
}
