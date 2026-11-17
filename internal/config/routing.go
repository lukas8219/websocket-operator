package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http/httpguts"
)

const UserHeaderEnv = "WS_OPERATOR_USER_HEADER"
const DefaultUserHeader = "ws-user-id"

// UserHeaderFromEnv validates the shared wire contract before a server starts.
func UserHeaderFromEnv() (string, error) {
	value, present := os.LookupEnv(UserHeaderEnv)
	if !present {
		return DefaultUserHeader, nil
	}
	if !httpguts.ValidHeaderFieldName(value) {
		return "", fmt.Errorf("%s must be a nonempty HTTP header name", UserHeaderEnv)
	}
	name := strings.ToLower(value)
	switch name {
	case "host", "connection", "upgrade", "content-type", "content-length", "transfer-encoding":
		return "", fmt.Errorf("%s cannot use protocol header %q", UserHeaderEnv, value)
	}
	if strings.HasPrefix(name, "sec-websocket-") {
		return "", fmt.Errorf("%s cannot use a WebSocket handshake header", UserHeaderEnv)
	}
	return http.CanonicalHeaderKey(value), nil
}
