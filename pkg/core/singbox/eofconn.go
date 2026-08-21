package singbox

import (
	"context"
	"errors"
	"io"
	"net"
)

// eofConn restores io.EOF identity on reads.
//
// sing-quic (>= v0.6.0-beta.6) wraps every stream-read error in its own
// *quicError, including a plain io.EOF. A QUIC stream returns (data, io.EOF)
// in one call when the peer sends the response together with the stream FIN,
// which is what a short reply like generate_204 looks like. The standard
// library compares against io.EOF by identity — bytes.Buffer.ReadFrom,
// crypto/tls's atLeastReader and net/http's readLoop all use ==, not
// errors.Is — so the wrapper makes a clean end-of-stream look like a
// transport failure and the response bytes that already arrived get
// discarded. Every hysteria2 config then fails with a bare "EOF".
//
// Unwrapping here fixes it for any sing-quic based protocol (hysteria2,
// TUIC) without pinning or forking sing-quic.
//
// Upstream: https://github.com/lilendian0x00/xray-knife/issues/66
type eofConn struct {
	net.Conn
}

func (c *eofConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err != nil && err != io.EOF && errors.Is(err, io.EOF) {
		err = io.EOF
	}
	return n, err
}

// dialFunc is the signature http.Transport.DialContext expects.
type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// withEOFNormalization wraps every conn a dialer returns in eofConn.
func withEOFNormalization(dial dialFunc) dialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &eofConn{Conn: conn}, nil
	}
}
