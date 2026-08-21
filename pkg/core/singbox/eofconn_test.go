package singbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

// quicError mimics sing-quic's wrapper: it reports io.EOF through errors.Is
// but is not io.EOF by identity, which is what the standard library checks.
type quicError struct{ err error }

func (e *quicError) Error() string { return fmt.Sprintf("quic: %s", e.err) }
func (e *quicError) Unwrap() error { return e.err }

type scriptedConn struct {
	net.Conn
	payload []byte
	err     error
	done    bool
}

func (c *scriptedConn) Read(b []byte) (int, error) {
	if c.done {
		return 0, c.err
	}
	c.done = true
	n := copy(b, c.payload)
	return n, c.err
}

func TestEOFConnRestoresEOFIdentity(t *testing.T) {
	wrapped := &quicError{err: io.EOF}
	if wrapped == io.EOF { //nolint:staticcheck // documents the failure mode
		t.Fatal("test setup is wrong: the wrapper must not be io.EOF by identity")
	}
	if !errors.Is(wrapped, io.EOF) {
		t.Fatal("test setup is wrong: the wrapper must report io.EOF via errors.Is")
	}

	c := &eofConn{Conn: &scriptedConn{payload: []byte("HTTP/1.1 204 No Content\r\n\r\n"), err: wrapped}}

	buf := make([]byte, 64)
	n, err := c.Read(buf)

	if n == 0 {
		t.Fatal("payload was dropped")
	}
	if err != io.EOF {
		t.Fatalf("Read err = %#v, want io.EOF by identity", err)
	}
}

func TestEOFConnLeavesOtherErrorsAlone(t *testing.T) {
	want := errors.New("connection reset by peer")
	c := &eofConn{Conn: &scriptedConn{payload: []byte("x"), err: want}}

	if _, err := c.Read(make([]byte, 4)); !errors.Is(err, want) {
		t.Fatalf("Read err = %v, want %v untouched", err, want)
	}
}

func TestEOFConnPassesPlainEOFThrough(t *testing.T) {
	c := &eofConn{Conn: &scriptedConn{payload: nil, err: io.EOF}}

	if _, err := c.Read(make([]byte, 4)); err != io.EOF {
		t.Fatalf("Read err = %#v, want io.EOF", err)
	}
}

func TestWithEOFNormalizationWrapsTheConn(t *testing.T) {
	base := &scriptedConn{payload: []byte("ok"), err: &quicError{err: io.EOF}}
	dial := withEOFNormalization(func(context.Context, string, string) (net.Conn, error) {
		return base, nil
	})

	conn, err := dial(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	if _, err := conn.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("Read err = %#v, want io.EOF", err)
	}
}

func TestWithEOFNormalizationPropagatesDialErrors(t *testing.T) {
	want := errors.New("no route to host")
	dial := withEOFNormalization(func(context.Context, string, string) (net.Conn, error) {
		return nil, want
	})

	conn, err := dial(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		t.Fatal("conn should be nil when the dial fails")
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
