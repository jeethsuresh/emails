//go:build js && wasm

package netbridge

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"syscall/js"
	"time"
)

type Security string

const (
	SecurityTLS      Security = "tls"
	SecuritySTARTTLS Security = "starttls"
	SecurityNone     Security = "none"
)

func Install() {
	js.Global().Set("emailNetInstalled", true)
}

func ParseSecurity(s string, legacyTLS bool) Security {
	switch Security(s) {
	case SecurityTLS, SecuritySTARTTLS, SecurityNone:
		return Security(s)
	default:
		if legacyTLS {
			return SecurityTLS
		}
		return SecurityNone
	}
}

func TLSConfig(serverName string, insecure bool) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: insecure,
		MinVersion:         tls.VersionTLS12,
	}
}

type jsConn struct {
	id     int
	local  net.Addr
	remote net.Addr
}

func Dial(network, address string, sec Security, insecure bool) (net.Conn, error) {
	_ = insecure
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	var p int
	_, _ = fmtSscanf(port, &p)

	useTLS := sec == SecurityTLS
	netObj := js.Global().Get("emailNet")
	done := make(chan struct {
		id  int
		err error
	}, 1)

	promise := netObj.Call("dial", host, p, useTLS)
	promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- struct {
			id  int
			err error
		}{id: args[0].Int()}
		return nil
	})).Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
		msg := "dial failed"
		if len(args) > 0 {
			msg = args[0].String()
		}
		done <- struct {
			id  int
			err error
		}{err: &net.OpError{Op: "dial", Net: network, Err: errString(msg)}}
		return nil
	}))

	res := <-done
	if res.err != nil {
		return nil, res.err
	}
	if sec == SecuritySTARTTLS {
		return nil, fmt.Errorf("STARTTLS over WASM net bridge is not implemented yet — use the native backend")
	}
	return &jsConn{
		id:     res.id,
		local:  dummyAddr("local"),
		remote: dummyAddr(address),
	}, nil
}

func (c *jsConn) Read(b []byte) (int, error) {
	netObj := js.Global().Get("emailNet")
	done := make(chan struct {
		data []byte
		err  error
	}, 1)

	promise := netObj.Call("read", c.id, len(b))
	promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		arr := args[0]
		n := arr.Get("byteLength").Int()
		if n == 0 {
			n = arr.Get("length").Int()
		}
		buf := make([]byte, n)
		if n > 0 {
			js.CopyBytesToGo(buf, arr)
		}
		var err error
		if n == 0 {
			err = io.EOF
		}
		done <- struct {
			data []byte
			err  error
		}{data: buf, err: err}
		return nil
	}))

	res := <-done
	if res.err != nil && len(res.data) == 0 {
		return 0, res.err
	}
	n := copy(b, res.data)
	return n, nil
}

func (c *jsConn) Write(b []byte) (int, error) {
	netObj := js.Global().Get("emailNet")
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	n := netObj.Call("write", c.id, arr).Int()
	return n, nil
}

func (c *jsConn) Close() error {
	js.Global().Get("emailNet").Call("close", c.id)
	return nil
}

func (c *jsConn) LocalAddr() net.Addr                { return c.local }
func (c *jsConn) RemoteAddr() net.Addr               { return c.remote }
func (c *jsConn) SetDeadline(t time.Time) error      { return nil }
func (c *jsConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *jsConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return "tcp" }
func (d dummyAddr) String() string  { return string(d) }

type errString string

func (e errString) Error() string { return string(e) }

func fmtSscanf(port string, p *int) (int, error) {
	n := 0
	for _, c := range port {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	*p = n
	return 1, nil
}
