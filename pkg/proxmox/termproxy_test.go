package proxmox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testAuth = "PVEAPIToken=root@pam!omni=secret"

func tlsClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	return tlsClientFor(t, Cluster{}, h)
}

// tlsClientFor serves h over TLS and returns a token client for it with
// cluster's console settings.
func tlsClientFor(t *testing.T, cluster Cluster, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	cluster.ID, cluster.URL, cluster.InsecureSkipVerify = "c", srv.URL, true
	cluster.TokenID, cluster.TokenSecret = "root@pam!omni", "secret"
	c, err := NewClient(cluster)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTermProxyPostsSerialAndDecodesPort(t *testing.T) {
	var gotPath, gotSerial string
	c := tlsClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != testAuth {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		gotPath, gotSerial = r.URL.Path, r.PostFormValue("serial")
		_, _ = io.WriteString(w, `{"data":{"port":"5901","ticket":"PVEVNC:t","user":"root@pam!omni","upid":"UPID:x"}}`)
	}))

	tp, err := c.termProxy(context.Background(), c.authorizeToken, TermTarget{Node: "pve1", GuestType: "qemu", VMID: 100, Serial: "serial1"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api2/json/nodes/pve1/qemu/100/termproxy" || gotSerial != "serial1" {
		t.Fatalf("request = %s serial=%q", gotPath, gotSerial)
	}
	if tp != (TermProxy{Port: 5901, Ticket: "PVEVNC:t", User: "root@pam!omni", UPID: "UPID:x"}) {
		t.Fatalf("termproxy = %+v", tp)
	}

	if _, err := c.termProxy(context.Background(), c.authorizeToken, TermTarget{Node: "pve1"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api2/json/nodes/pve1/termproxy" || gotSerial != "" {
		t.Fatalf("node request = %s serial=%q", gotPath, gotSerial)
	}
}

// terminalServer answers termproxy and runs ws for each websocket.
func terminalServer(t *testing.T, ws func(ctx context.Context, conn *websocket.Conn)) *Client {
	t.Helper()
	return tlsClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != testAuth {
			http.Error(w, "authentication failure", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/termproxy"):
			_, _ = io.WriteString(w, `{"data":{"port":5900,"ticket":"PVEVNC:a+b/c=","user":"root@pam!omni","upid":"UPID:x"}}`)
		case strings.HasSuffix(r.URL.Path, "/vncwebsocket"):
			if q := r.URL.Query(); q.Get("port") != "5900" || q.Get("vncticket") != "PVEVNC:a+b/c=" {
				http.Error(w, "bad ticket", http.StatusBadRequest)
				return
			}
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"binary"}})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			if conn.Subprotocol() != "binary" {
				t.Errorf("subprotocol = %q", conn.Subprotocol())
			}
			ws(r.Context(), conn)
		default:
			http.NotFound(w, r)
		}
	}))
}

func readMessage(t *testing.T, ctx context.Context, conn *websocket.Conn) string {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Errorf("server read: %v", err)
		return ""
	}
	return string(data)
}

func TestTerminalSpeaksTermproxyProtocol(t *testing.T) {
	serverDone := make(chan []string, 1)
	c := terminalServer(t, func(ctx context.Context, conn *websocket.Conn) {
		got := []string{readMessage(t, ctx, conn)}
		_ = conn.Write(ctx, websocket.MessageBinary, []byte("OKbanner"))
		for len(got) < 4 {
			got = append(got, readMessage(t, ctx, conn))
		}
		serverDone <- got
		_ = conn.Write(ctx, websocket.MessageBinary, []byte("out"))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	target := TermTarget{Node: "pve1", GuestType: "lxc", VMID: 101}
	tp, err := c.termProxy(context.Background(), c.authorizeToken, target)
	if err != nil {
		t.Fatal(err)
	}
	// The keepalive must come after the input and resize frames below.
	term, err := c.dialTerminal(context.Background(), c.authorizeToken, target, tp, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	buf := make([]byte, 64)
	n, err := term.Read(buf)
	if err != nil || string(buf[:n]) != "banner" {
		t.Fatalf("first read = %q, %v", buf[:n], err)
	}
	if _, err := term.Write([]byte("é\n")); err != nil {
		t.Fatal(err)
	}
	if err := term.Resize(120, 40); err != nil {
		t.Fatal(err)
	}

	var got []string
	select {
	case got = <-serverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish")
	}
	want := []string{"root@pam!omni:PVEVNC:a+b/c=\n", "0:3:é\n", "1:120:40:", "2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message %d = %q, want %q (all: %q)", i, got[i], want[i], got)
		}
	}

	out, err := io.ReadAll(term)
	if err != nil || string(out) != "out" {
		t.Fatalf("rest = %q, %v; want \"out\" then EOF", out, err)
	}
}

func TestTerminalHandshakeRejected(t *testing.T) {
	c := terminalServer(t, func(ctx context.Context, conn *websocket.Conn) {
		readMessage(t, ctx, conn)
		_ = conn.Write(ctx, websocket.MessageBinary, []byte("NO\n"))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	_, err := c.OpenTerminal(context.Background(), TermTarget{Node: "pve1"})
	if !errors.Is(err, ErrTerminalHandshake) {
		t.Fatalf("err = %v, want ErrTerminalHandshake", err)
	}
}

func TestOpenTerminalReportsWebsocketStatus(t *testing.T) {
	c := tlsClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/termproxy") {
			_, _ = io.WriteString(w, `{"data":{"port":5900,"ticket":"t","user":"u","upid":"x"}}`)
			return
		}
		http.Error(w, "Permission check failed (/vms/100, VM.Console)", http.StatusForbidden)
	}))
	_, err := c.OpenTerminal(context.Background(), TermTarget{Node: "pve1", GuestType: "qemu", VMID: 100, Serial: "serial0"})
	if !IsStatus(err, http.StatusForbidden) {
		t.Fatalf("err = %v, want 403 APIError", err)
	}
}
