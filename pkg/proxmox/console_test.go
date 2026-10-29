package proxmox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

func TestConsoleCredentialsValidation(t *testing.T) {
	const base = `{id: a, url: "https://pve:8006", token_id: "root@pam!t", token_secret: s`
	invalid := map[string]string{
		"user without password": base + `, console_user: root@pam}`,
		"password without user": base + `, console_password: hunter2}`,
		"file without user":     base + `, console_password_file: /tmp/pw}`,
		"both password sources": base + `, console_user: root@pam, console_password: hunter2, console_password_file: /tmp/pw}`,
		"token as console user": base + `, console_user: "root@pam!t", console_password: hunter2}`,
		"user without realm":    base + `, console_user: root, console_password: hunter2}`,
	}
	for name, entry := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := LoadFile(writeConfig(t, "clusters:\n  - "+entry))
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Fatalf("error exposes the password: %v", err)
			}
		})
	}

	for name, entry := range map[string]string{
		"none":     base + `}`,
		"password": base + `, console_user: root@pam, console_password: hunter2}`,
		"file":     base + `, console_user: omniview@pve, console_password_file: ~/pw}`,
	} {
		t.Run("valid "+name, func(t *testing.T) {
			if _, err := LoadFile(writeConfig(t, "clusters:\n  - "+entry)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

const (
	consoleTicket = "PVE:root@pam:6720ABCD::c2ln+bmF/dHVyZQ=="
	consoleCSRF   = "6720ABCD:csrf"
)

// consoleServer is a fake PVE that only accepts the console user's ticket
// on termproxy and vncwebsocket. ticketReply answers /access/ticket.
func consoleServer(t *testing.T, ticketReply string, handshake chan<- string) *Client {
	t.Helper()
	pwFile := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(pwFile, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := tlsClientFor(t, Cluster{ConsoleUser: "root@pam", ConsolePasswordFile: pwFile}, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Errorf("%s %s sent the API token", r.Method, r.URL.Path)
			}
			switch {
			case r.URL.Path == "/api2/json/access/ticket":
				if r.Method != http.MethodPost || r.PostFormValue("username") != "root@pam" || r.PostFormValue("password") != "hunter2" {
					http.Error(w, "authentication failure", http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, ticketReply)
				return
			}
			cookie, err := r.Cookie("PVEAuthCookie")
			if err != nil || cookie.Value != consoleTicket {
				http.Error(w, "no ticket", http.StatusUnauthorized)
				return
			}
			switch {
			case strings.HasSuffix(r.URL.Path, "/termproxy"):
				if r.Header.Get("CSRFPreventionToken") != consoleCSRF {
					http.Error(w, "Permission denied - invalid csrf token", http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, `{"data":{"port":5900,"ticket":"PVEVNC:vt","user":"root@pam","upid":"UPID:x"}}`)
			case strings.HasSuffix(r.URL.Path, "/vncwebsocket"):
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"binary"}})
				if err != nil {
					t.Errorf("accept: %v", err)
					return
				}
				_, hello, _ := conn.Read(r.Context())
				handshake <- string(hello)
				_ = conn.Write(r.Context(), websocket.MessageBinary, []byte("OK"))
				_ = conn.Close(websocket.StatusNormalClosure, "")
			default:
				http.NotFound(w, r)
			}
		}))
	return c
}

func TestOpenTerminalUsesConsoleTicket(t *testing.T) {
	handshake := make(chan string, 1)
	reply := `{"data":{"ticket":"` + consoleTicket + `","CSRFPreventionToken":"` + consoleCSRF + `","username":"root@pam"}}`
	c := consoleServer(t, reply, handshake)
	if c.ConsoleUser() != "root@pam" {
		t.Fatalf("ConsoleUser = %q", c.ConsoleUser())
	}

	term, err := c.OpenTerminal(context.Background(), TermTarget{Node: "pve1"})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if got := <-handshake; got != "root@pam:PVEVNC:vt\n" {
		t.Fatalf("handshake = %q", got)
	}
	if _, err := io.ReadAll(term); err != nil {
		t.Fatalf("read: %v", err)
	}
}

func TestConsoleLoginRejectsTFAUser(t *testing.T) {
	reply := `{"data":{"ticket":"PVE:!tfa!x","CSRFPreventionToken":"c","username":"root@pam","NeedTFA":1}}`
	c := consoleServer(t, reply, nil)
	_, err := c.OpenTerminal(context.Background(), TermTarget{Node: "pve1"})
	if !errors.Is(err, ErrConsoleTFA) || !errors.Is(err, ErrConsoleLogin) {
		t.Fatalf("err = %v, want ErrConsoleTFA", err)
	}
}

func TestConsoleLoginFailureHidesPassword(t *testing.T) {
	c := tlsClientFor(t, Cluster{ConsoleUser: "root@pam", ConsolePassword: "wrong-pass"}, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "authentication failure", http.StatusUnauthorized)
		}))
	_, err := c.OpenTerminal(context.Background(), TermTarget{Node: "pve1"})
	if !errors.Is(err, ErrConsoleLogin) || !IsStatus(err, http.StatusUnauthorized) {
		t.Fatalf("err = %v, want a 401 console login error", err)
	}
	if strings.Contains(err.Error(), "wrong-pass") {
		t.Fatalf("error exposes the password: %v", err)
	}
}
