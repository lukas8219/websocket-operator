package utils

import (
	"bufio"
	"net"
)

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

// WithBufferedReader preserves bytes prefetched while parsing an HTTP upgrade.
// A buffer may contain only part of the first frame, so dropping it corrupts
// the stream. Keep the reader for the lifetime of this connection.
func WithBufferedReader(conn net.Conn, reader *bufio.Reader) net.Conn {
	if reader == nil {
		return conn
	}
	return &bufferedConn{Conn: conn, reader: reader}
}
