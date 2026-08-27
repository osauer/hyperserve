package hyperserve

import (
	"errors"
	"log/slog"
)

// WithLogger gives one Server its logger without changing slog's process-wide
// default. Configure the handler's level in the application when supplying a
// custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(srv *Server) error {
		if l == nil {
			return errors.New("hyperserve: nil logger")
		}
		srv.logger = l
		srv.customLogger = true
		return nil
	}
}
