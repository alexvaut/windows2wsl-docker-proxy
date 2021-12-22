package proxy

import (
	"bufio"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

func isComplete(content string, isRequest bool) (bool, bool) {

	reader := bufio.NewReader(strings.NewReader(content))
	var complete bool
	var chunck bool = false

	if isRequest {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return false, false
		}
		complete, chunck = test(req.TransferEncoding, req.ContentLength, req.Body)

	} else {
		res, err := http.ReadResponse(reader, nil)
		if err != nil {
			return false, false
		}
		complete, chunck = test(res.TransferEncoding, res.ContentLength, res.Body)
	}

	return complete, chunck

}

func test(TransferEncoding []string, ContentLength int64, body io.ReadCloser) (bool, bool) {
	isChuncked := TransferEncoding != nil && len(TransferEncoding) > 0 && TransferEncoding[0] == "chunked"

	buff := make([]byte, 0xfffff)
	if !isChuncked {
		n, err := body.Read(buff)
		switch err {
		case io.EOF:
			return ContentLength <= int64(n), false
		case nil:
			return ContentLength <= int64(n), false
		default:
			return false, false
		}

	}

	return true, true
}

func isLastChunkComplete(content string) (bool, bool) {

	start := 0
	if strings.HasPrefix(content, "HTTP") {
		start = getHttpPayloadStart(content)
		if start < 0 {
			start = 0
		}
	}

	payload := content[start:]

	for {
		//find chunk size
		index := strings.Index(payload, "\r\n")
		if index < 0 {
			return true, false
		}
		payloadStart := index + 2
		chunkSizeS := payload[0 : payloadStart-2]
		chunkSize, _ := strconv.ParseInt(chunkSizeS, 16, 64)

		if chunkSize == 0 {
			return true, true
		}

		start := payloadStart + int(chunkSize) + 2

		if start >= len(payload) {
			return false, false
		} else {
			payload = payload[start:]
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
