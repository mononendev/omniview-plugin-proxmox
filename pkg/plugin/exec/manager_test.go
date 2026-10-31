package exec

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/omniviewdev/plugin-sdk/pkg/types"
	sdkexec "github.com/omniviewdev/plugin-sdk/pkg/v1/exec"
	"github.com/omniviewdev/plugin-sdk/pkg/v1/exec/exectest"
)

// managerSession opens a node shell through the SDK's exec Manager with
// real PTYs, as the host does, and waits until the console is bridged.
func managerSession(t *testing.T, ws func(ctx context.Context, conn *websocket.Conn)) (*sdkexec.Manager, *exectest.RecordingOutput, string) {
	t.Helper()
	f := &fakePVE{t: t, ws: ws}
	byKey := map[string]sdkexec.Handler{}
	for _, h := range handlers(f.clients()) {
		byKey[h.ID()] = h
	}
	out := exectest.NewRecordingOutput()
	mgr := sdkexec.NewManager(sdkexec.ManagerConfig{Handlers: byKey, Sink: out})
	t.Cleanup(mgr.Close)

	pctx := &types.PluginContext{Context: context.Background()}
	sess, err := mgr.CreateResourceSession(pctx, sdkexec.SessionOptions{
		ResourcePlugin: pluginID, ResourceKey: keyNode,
		ResourceData: body(map[string]any{"id": "node/ares", "node": "ares"}), TTY: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mgr.AttachSession(pctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !bytes.Contains(out.AllData(), []byte("ready")) {
		if time.Now().After(deadline) {
			t.Fatalf("console output never reached the manager: %q", out.AllData())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return mgr, out, sess.ID
}

func TestManagerCloseSessionReturnsPromptly(t *testing.T) {
	mgr, _, id := managerSession(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageBinary, []byte("ready"))
		_, _, _ = conn.Read(ctx) // until the plugin closes
	})

	start := time.Now()
	if err := mgr.CloseSession(&types.PluginContext{Context: context.Background()}, id); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("CloseSession took %v", d)
	}
}

func TestManagerSessionEndsWhenServerCloses(t *testing.T) {
	closeNow := make(chan struct{})
	mgr, out, id := managerSession(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageBinary, []byte("ready"))
		<-closeNow
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	start := time.Now()
	close(closeNow)
	if out.WaitForSignal(sdkexec.StreamSignalClose, 2*time.Second) == nil {
		t.Fatal("no close signal after the server closed")
	}
	// All of the session's goroutines must finish too: the manager waits
	// for them before reporting the session gone.
	if err := mgr.CloseSession(&types.PluginContext{Context: context.Background()}, id); err == nil {
		t.Fatal("session still registered after it ended")
	}
	done := make(chan struct{})
	go func() { mgr.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2*time.Second - time.Since(start)):
		t.Fatal("manager goroutines still blocked after the session ended")
	}
}

func TestManagerCloseSessionWhileConnecting(t *testing.T) {
	requested := make(chan struct{})
	f := &fakePVE{t: t, termproxy: func(_ http.ResponseWriter, r *http.Request) {
		close(requested)
		<-r.Context().Done() // never answers
	}}
	byKey := map[string]sdkexec.Handler{}
	for _, h := range handlers(f.clients()) {
		byKey[h.ID()] = h
	}
	mgr := sdkexec.NewManager(sdkexec.ManagerConfig{Handlers: byKey, Sink: exectest.NewRecordingOutput()})
	t.Cleanup(mgr.Close)
	pctx := &types.PluginContext{Context: context.Background()}
	sess, err := mgr.CreateResourceSession(pctx, sdkexec.SessionOptions{
		ResourcePlugin: pluginID, ResourceKey: keyNode,
		ResourceData: body(map[string]any{"id": "node/ares"}), TTY: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-requested

	start := time.Now()
	if err := mgr.CloseSession(pctx, sess.ID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("CloseSession took %v", d)
	}
}
