package hyperserve

import pkgwebsocket "github.com/osauer/hyperserve/pkg/websocket"

type (
	WebSocketPool = pkgwebsocket.WebSocketPool
	PoolConfig    = pkgwebsocket.PoolConfig
	PoolStats     = pkgwebsocket.PoolStats
)

func DefaultPoolConfig() PoolConfig { return pkgwebsocket.DefaultPoolConfig() }

func NewWebSocketPool(config PoolConfig) *WebSocketPool {
	return pkgwebsocket.NewWebSocketPool(config)
}
