package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// TerminalKeepalive is how often an open terminal pings the termproxy, as the
// Proxmox web UI does; termproxy drops idle connections.
const TerminalKeepalive = 30 * time.Second

// TermTarget selects the console a terminal attaches to: the node shell when
// GuestType is empty, otherwise the console of guest VMID ("qemu" or "lxc").
type TermTarget struct {
	Node      string
	GuestType string
	VMID      int
	// Serial is the qemu serial interface (serial0..serial3); qemu only.
	Serial string
}

func (t TermTarget) path() string {
	p := "/nodes/" + url.PathEscape(t.Node)
	if t.GuestType != "" {
		p += "/" + t.GuestType + "/" + strconv.Itoa(t.VMID)
	}
	return p
}

// TermProxy is the POST .../termproxy payload: a one-shot ticket for the
// console process listening on Port.
type TermProxy struct {
	Port   int
	Ticket string
	User   string
	UPID   string
}

func (tp *TermProxy) UnmarshalJSON(b []byte) error {
	var raw struct {
		Port   json.RawMessage `json:"port"`
		Ticket string          `json:"ticket"`
		User   string          `json:"user"`
		UPID   string          `json:"upid"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	// PVE serializes the port as a number or a numeric string.
	port, err := strconv.Atoi(strings.Trim(string(raw.Port), `"`))
	if err != nil {
		return fmt.Errorf("termproxy port %s: %w", raw.Port, err)
	}
	*tp = TermProxy{Port: port, Ticket: raw.Ticket, User: raw.User, UPID: raw.UPID}
	return nil
}

// termProxy starts a console process for target.
func (c *Client) termProxy(ctx context.Context, authorize func(*http.Request), target TermTarget) (TermProxy, error) {
	form := url.Values{}
	if target.GuestType == "qemu" && target.Serial != "" {
		form.Set("serial", target.Serial)
	}
	var tp TermProxy
	err := c.send(ctx, authorize, http.MethodPost, target.path()+"/termproxy", nil, form, &tp)
	return tp, err
}

// Terminal is an authenticated pve-xtermjs websocket. Read yields the raw
// console output; Write sends input. Close ends the console.
type Terminal struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	// pending holds output received with the handshake reply.
	pending []byte
	reader  io.Reader

	closeOnce sync.Once
	done      chan struct{}
}

// OpenTerminal starts a console for target and connects to it: termproxy,
// vncwebsocket, then the `user:ticket` handshake. With a console user
// configured, it first logs that user in and authenticates both requests
// with the login ticket (PVE consoles reject API tokens); otherwise it
// tries the token. ctx bounds only the setup; the terminal lives until
// Close or the server ends it.
func (c *Client) OpenTerminal(ctx context.Context, target TermTarget) (*Terminal, error) {
	authorize := c.authorizeToken
	if c.console.user != "" {
		session, err := c.loginConsole(ctx)
		if err != nil {
			return nil, err
		}
		authorize = session.authorize
	}
	tp, err := c.termProxy(ctx, authorize, target)
	if err != nil {
		return nil, err
	}
	return c.dialTerminal(ctx, authorize, target, tp, TerminalKeepalive)
}

func (c *Client) dialTerminal(
	ctx context.Context,
	authorize func(*http.Request),
	target TermTarget,
	tp TermProxy,
	keepalive time.Duration,
) (*Terminal, error) {
	u, err := c.endpoint(target.path() + "/vncwebsocket")
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	u.RawQuery = url.Values{"port": {strconv.Itoa(tp.Port)}, "vncticket": {tp.Ticket}}.Encode()

	// The handshake request is a GET: authorize sets the token header or
	// the ticket cookie, never a CSRF token.
	headers, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	authorize(headers)

	conn, resp, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPClient:   c.websocketHTTPClient(),
		HTTPHeader:   headers.Header,
		Subprotocols: []string{"binary"},
	})
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			reason := strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
			return nil, &APIError{StatusCode: resp.StatusCode, Reason: reason}
		}
		return nil, fmt.Errorf("connect terminal websocket: %w", err)
	}
	conn.SetReadLimit(-1)

	pending, err := terminalHandshake(ctx, conn, tp)
	if err != nil {
		conn.CloseNow()
		return nil, err
	}

	lifetime, cancel := context.WithCancel(context.Background())
	t := &Terminal{conn: conn, ctx: lifetime, cancel: cancel, pending: pending, done: make(chan struct{})}
	go t.keepalive(keepalive)
	return t, nil
}

// websocketHTTPClient shares the API client's TLS settings but speaks
// HTTP/1.1 only: the websocket upgrade does not exist in HTTP/2.
func (c *Client) websocketHTTPClient() *http.Client {
	base, ok := c.http.Transport.(*http.Transport)
	if !ok {
		return c.http
	}
	transport := base.Clone()
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	}
	return &http.Client{Transport: transport}
}

// ErrTerminalHandshake reports that termproxy did not accept the console
// ticket: it answered something other than "OK" or closed the connection.
var ErrTerminalHandshake = errors.New("terminal handshake failed")

// terminalHandshake authenticates to termproxy and returns any output that
// arrived with its "OK".
func terminalHandshake(ctx context.Context, conn *websocket.Conn, tp TermProxy) ([]byte, error) {
	if err := conn.Write(ctx, websocket.MessageBinary, []byte(tp.User+":"+tp.Ticket+"\n")); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTerminalHandshake, err)
	}
	_, reply, err := conn.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTerminalHandshake, err)
	}
	if !bytes.HasPrefix(reply, []byte("OK")) {
		return nil, fmt.Errorf("%w: termproxy answered %q", ErrTerminalHandshake, bytes.TrimSpace(reply))
	}
	return reply[2:], nil
}

// Read returns console output. It returns io.EOF once the server closes the
// console.
func (t *Terminal) Read(p []byte) (int, error) {
	if len(t.pending) > 0 {
		n := copy(p, t.pending)
		t.pending = t.pending[n:]
		return n, nil
	}
	for {
		if t.reader == nil {
			_, r, err := t.conn.Reader(t.ctx)
			if err != nil {
				return 0, terminalReadError(err)
			}
			t.reader = r
		}
		n, err := t.reader.Read(p)
		if errors.Is(err, io.EOF) {
			t.reader = nil
			if n == 0 {
				continue
			}
			err = nil
		}
		if err != nil {
			return n, terminalReadError(err)
		}
		return n, nil
	}
}

func terminalReadError(err error) error {
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return io.EOF
	}
	if errors.Is(err, io.EOF) {
		return io.EOF
	}
	return err
}

// Write sends p as one input frame: `0:<bytelen>:<data>`.
func (t *Terminal) Write(p []byte) (int, error) {
	frame := make([]byte, 0, len(p)+16)
	frame = append(frame, "0:"...)
	frame = strconv.AppendInt(frame, int64(len(p)), 10)
	frame = append(frame, ':')
	frame = append(frame, p...)
	if err := t.conn.Write(t.ctx, websocket.MessageBinary, frame); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize sends the terminal size: `1:<cols>:<rows>:`.
func (t *Terminal) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("invalid terminal size %dx%d", cols, rows)
	}
	frame := fmt.Appendf(nil, "1:%d:%d:", cols, rows)
	return t.conn.Write(t.ctx, websocket.MessageBinary, frame)
}

func (t *Terminal) keepalive(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			if err := t.conn.Write(t.ctx, websocket.MessageBinary, []byte("2")); err != nil {
				return
			}
		}
	}
}

// Close sends a normal closure and releases the connection. It is safe to
// call more than once.
func (t *Terminal) Close() error {
	var err error
	t.closeOnce.Do(func() {
		close(t.done)
		err = t.conn.Close(websocket.StatusNormalClosure, "")
		t.cancel()
	})
	if errors.Is(err, net.ErrClosed) || websocket.CloseStatus(err) != -1 {
		// The peer closed first; the closure is already complete.
		return nil
	}
	return err
}
