package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isComplete_NoBodyButRequied(t *testing.T) {

	content := `POST /v1.41/containers/create?name=empirix-authn-management_devcontainer_keycloak_1 HTTP/1.1
Host: localhost:2376
User-Agent: docker-compose/1.29.2 docker-py/5.0.0 Windows/10
Accept-Encoding: gzip, deflate
Accept: */*
Connection: keep-alive
Content-Type: application/json
Content-Length: 1394

`

	complete, chunk := isComplete(content, true)
	assert.False(t, complete, "Body is missing, should be false")
	assert.False(t, chunk, "Not chunked, should be false")
}

func Test_isComplete_BodyAndRequired(t *testing.T) {

	content := `POST /v1.41/containers/create?name=empirix-authn-management_devcontainer_keycloak_1 HTTP/1.1
Host: localhost:2376
User-Agent: docker-compose/1.29.2 docker-py/5.0.0 Windows/10
Accept-Encoding: gzip, deflate
Accept: */*
Connection: keep-alive
Content-Type: application/json
Content-Length: 4

0000
`

	complete, chunk := isComplete(content, true)
	assert.True(t, complete, "Body is missing, should be false")
	assert.False(t, chunk, "Not chunked, should be false")
}

func Test_isComplete_NoBodyNotRequired(t *testing.T) {

	content := `HEAD /_ping HTTP/1.1
Host: localhost:2376
User-Agent: Docker-Client/19.03.12 (windows)


`
	complete, chunk := isComplete(content, true)
	assert.True(t, complete, "No Body required, should be true")
	assert.False(t, chunk, "Not chunked, should be false")
}
