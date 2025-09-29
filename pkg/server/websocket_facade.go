package server

import (
	"net/http"

	pkgwebsocket "github.com/osauer/hyperserve/pkg/websocket"
)

type (
	Upgrader = pkgwebsocket.Upgrader
	Conn     = pkgwebsocket.Conn
)

const (
	TextMessage                  = pkgwebsocket.TextMessage
	BinaryMessage                = pkgwebsocket.BinaryMessage
	CloseMessage                 = pkgwebsocket.CloseMessage
	PingMessage                  = pkgwebsocket.PingMessage
	PongMessage                  = pkgwebsocket.PongMessage
	CloseNormalClosure           = pkgwebsocket.CloseNormalClosure
	CloseGoingAway               = pkgwebsocket.CloseGoingAway
	CloseProtocolError           = pkgwebsocket.CloseProtocolError
	CloseUnsupportedData         = pkgwebsocket.CloseUnsupportedData
	CloseNoStatusReceived        = pkgwebsocket.CloseNoStatusReceived
	CloseAbnormalClosure         = pkgwebsocket.CloseAbnormalClosure
	CloseInvalidFramePayloadData = pkgwebsocket.CloseInvalidFramePayloadData
	ClosePolicyViolation         = pkgwebsocket.ClosePolicyViolation
	CloseMessageTooBig           = pkgwebsocket.CloseMessageTooBig
	CloseMandatoryExtension      = pkgwebsocket.CloseMandatoryExtension
	CloseInternalServerError     = pkgwebsocket.CloseInternalServerError
	CloseServiceRestart          = pkgwebsocket.CloseServiceRestart
	CloseTryAgainLater           = pkgwebsocket.CloseTryAgainLater
	CloseTLSHandshake            = pkgwebsocket.CloseTLSHandshake
)

var (
	ErrNotWebSocket = pkgwebsocket.ErrNotWebSocket
	ErrBadHandshake = pkgwebsocket.ErrBadHandshake
)

// DefaultCheckOrigin wraps pkg/websocket.DefaultCheckOrigin for internal use.
func DefaultCheckOrigin(r *http.Request) bool {
	return pkgwebsocket.DefaultCheckOrigin(r)
}

// CheckOriginWithAllowedList wraps pkg/websocket.CheckOriginWithAllowedList.
func CheckOriginWithAllowedList(allowedOrigins []string) func(r *http.Request) bool {
	return pkgwebsocket.CheckOriginWithAllowedList(allowedOrigins)
}

func IsCloseError(err error, codes ...int) bool {
	return pkgwebsocket.IsCloseError(err, codes...)
}

func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	return pkgwebsocket.IsUnexpectedCloseError(err, expectedCodes...)
}
