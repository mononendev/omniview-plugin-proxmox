package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"
	sdklogs "github.com/omniviewdev/plugin-sdk/pkg/v1/logs"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// body is a resource body as the host passes it.
func body(fields map[string]any) *resourcev2.Value {
	v, err := resourcev2.ValueFromNative(fields)
	if err != nil {
		panic(err)
	}
	return &v
}

// fakeJournal is a node journal behind GET /nodes/ares/syslog with the
// API's semantics: since filters at second resolution (node-local wall
// clock), start/limit page the result and total counts it.
type fakeJournal struct {
	mu        sync.Mutex
	t         *testing.T
	entries   []entry
	queries   []url.Values
	forbidden bool
}

type entry struct {
	at   time.Time
	unit string
	msg  string
}

var zone = time.FixedZone("EDT", -4*3600)

func (f *fakeJournal) add(at time.Time, unit, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry{at, unit, msg})
}

func (f *fakeJournal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api2/json/nodes/ares/time":
		now := time.Now()
		_, offset := now.In(zone).Zone()
		_, _ = w.Write([]byte(`{"data":{"time":` + strconv.FormatInt(now.Unix(), 10) +
			`,"localtime":` + strconv.FormatInt(now.Unix()+int64(offset), 10) + `,"timezone":"Nowhere/Fake"}}`))
		return
	case "/api2/json/nodes/ares/syslog":
	default:
		f.t.Errorf("unexpected request %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if f.forbidden {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	f.queries = append(f.queries, q)

	var since time.Time
	if s := q.Get("since"); s != "" {
		var err error
		if since, err = time.ParseInLocation(proxmox.SyslogTimeLayout, s, zone); err != nil {
			f.t.Errorf("bad since %q", s)
		}
	}
	var lines []string
	for _, e := range f.entries {
		if (q.Get("service") != "" && e.unit != q.Get("service")) || e.at.Truncate(time.Second).Before(since) {
			continue
		}
		lines = append(lines, e.at.In(zone).Format(time.Stamp)+" ares "+e.unit+"[1]: "+e.msg)
	}
	start, _ := strconv.Atoi(q.Get("start"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 50
	}
	type line struct {
		N int    `json:"n"`
		T string `json:"t"`
	}
	data := []line{}
	for i := start; i < len(lines) && len(data) < limit; i++ {
		data = append(data, line{i + 1, lines[i]})
	}
	total := len(lines)
	if total == 0 {
		data, total = []line{{1, "-- No entries --"}}, 1
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "total": total})
}

func (f *fakeJournal) lastQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func newJournal(t *testing.T) (*fakeJournal, *journal) {
	t.Helper()
	f := &fakeJournal{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	return f, &journal{client: client, node: "ares", unit: "ceph-osd@19", loc: zone, interval: 20 * time.Millisecond}
}

// collect reads lines from r into a channel until it ends.
func collect(r io.Reader) <-chan string {
	out := make(chan string, 100)
	go func() {
		defer close(out)
		s := bufio.NewScanner(r)
		for s.Scan() {
			out <- s.Text()
		}
	}()
	return out
}

func expect(t *testing.T, lines <-chan string, want ...string) {
	t.Helper()
	for _, w := range want {
		select {
		case got, ok := <-lines:
			if !ok {
				t.Fatalf("stream ended, want %q", w)
			}
			if !strings.HasSuffix(got, " ares ceph-osd@19[1]: "+w) {
				t.Fatalf("got line %q, want message %q", got, w)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %q", w)
		}
	}
}

func expectQuiet(t *testing.T, lines <-chan string) {
	t.Helper()
	select {
	case got := <-lines:
		t.Fatalf("unexpected line %q", got)
	case <-time.After(150 * time.Millisecond): // several polls
	}
}

func TestBacklogTailsTheUnitWithTimestamps(t *testing.T) {
	f, j := newJournal(t)
	base := time.Now().Truncate(time.Second).Add(-time.Minute)
	for i := range 5 {
		f.add(base.Add(time.Duration(i)*time.Second), "ceph-osd@19", "osd "+strconv.Itoa(i))
		f.add(base.Add(time.Duration(i)*time.Second), "ceph-mon@ares", "mon "+strconv.Itoa(i))
	}

	r, err := j.stream(context.Background(), sdklogs.LogStreamRequest{TailLines: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := collect(r)
	first := <-lines
	if want := base.Add(3*time.Second).In(zone).Format(time.RFC3339) + " ares ceph-osd@19[1]: osd 3"; first != want {
		t.Errorf("first line %q, want %q", first, want)
	}
	expect(t, lines, "osd 4")
	if _, open := <-lines; open {
		t.Error("non-follow stream did not end")
	}
	q := f.lastQuery()
	if q.Get("service") != "ceph-osd@19" || q.Get("since") == "" {
		t.Errorf("query %v", q)
	}
}

func TestBacklogSinceHonoursTheStartTime(t *testing.T) {
	f, j := newJournal(t)
	base := time.Now().Truncate(time.Second).Add(-time.Minute)
	for i := range 4 {
		f.add(base.Add(time.Duration(i)*10*time.Second), "ceph-osd@19", "osd "+strconv.Itoa(i))
	}
	since := base.Add(15 * time.Second)
	r, err := j.stream(context.Background(), sdklogs.LogStreamRequest{SinceTime: &since})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := collect(r)
	expect(t, lines, "osd 2", "osd 3")
	if _, open := <-lines; open {
		t.Error("non-follow stream did not end")
	}
	if got, want := f.lastQuery().Get("since"), since.In(zone).Format(proxmox.SyslogTimeLayout); got != want {
		t.Errorf("since = %q, want %q", got, want)
	}
}

func TestBacklogPagesLargeResults(t *testing.T) {
	f, j := newJournal(t)
	base := time.Now().Add(-30 * time.Minute)
	n := pageSize*2 + 10
	for i := range n {
		f.add(base.Add(time.Duration(i)*time.Millisecond), "ceph-osd@19", "osd "+strconv.Itoa(i))
	}
	r, err := j.stream(context.Background(), sdklogs.LogStreamRequest{TailLines: int64(pageSize + 5)})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	count := 0
	for line := range collect(r) {
		if want := "osd " + strconv.Itoa(n-pageSize-5+count); !strings.HasSuffix(line, ": "+want) {
			t.Fatalf("line %d = %q, want %q", count, line, want)
		}
		count++
	}
	if count != pageSize+5 {
		t.Errorf("read %d lines, want %d", count, pageSize+5)
	}
}

func TestBacklogWidensTheWindowUntilTheTailIsFound(t *testing.T) {
	f, j := newJournal(t)
	old := time.Now().Add(-3 * 24 * time.Hour)
	f.add(old, "ceph-osd@19", "old")
	f.add(time.Now().Add(-time.Minute), "ceph-osd@19", "recent")
	r, err := j.stream(context.Background(), sdklogs.LogStreamRequest{TailLines: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := collect(r)
	expect(t, lines, "old", "recent")
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) != 3 {
		t.Errorf("%d syslog queries, want hour, day and week windows", len(f.queries))
	}
}

func TestFollowDeliversEachLineOnce(t *testing.T) {
	f, j := newJournal(t)
	now := time.Now().Truncate(time.Second)
	prev := now.Add(-time.Second)
	f.add(now.Add(-10*time.Second), "ceph-osd@19", "a")
	f.add(prev, "ceph-osd@19", "b")
	f.add(prev.Add(100*time.Millisecond), "ceph-osd@19", "c")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := j.stream(ctx, sdklogs.LogStreamRequest{Follow: true, TailLines: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := collect(r)
	expect(t, lines, "b", "c")
	expectQuiet(t, lines)

	// A late line in the cursor's second, repeating an earlier text, and a
	// line in a new second.
	f.add(prev.Add(500*time.Millisecond), "ceph-osd@19", "c")
	f.add(now, "ceph-osd@19", "d")
	f.add(now, "ceph-mon@ares", "other unit")
	expect(t, lines, "c", "d")
	expectQuiet(t, lines)

	f.add(now.Add(time.Second), "ceph-osd@19", "e")
	f.add(now.Add(time.Second), "ceph-osd@19", "e")
	expect(t, lines, "e", "e")
	expectQuiet(t, lines)

	if since := f.lastQuery().Get("since"); since != now.Add(time.Second).In(zone).Format(proxmox.SyslogTimeLayout) {
		t.Errorf("follow polls since %q, want the newest second", since)
	}

	cancel()
	select {
	case _, open := <-lines:
		if open {
			t.Error("line after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end on cancel")
	}
}

func TestFollowAnEmptyJournal(t *testing.T) {
	f, j := newJournal(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := j.stream(ctx, sdklogs.LogStreamRequest{Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := collect(r)
	// A daemon without any journal says so instead of showing nothing.
	select {
	case got := <-lines:
		if !strings.HasSuffix(got, "No journal entries for ceph-osd@19 on ares in the last 30 days.") {
			t.Fatalf("first line %q, want the no-entries notice", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notice for an empty journal")
	}
	expectQuiet(t, lines)
	f.add(time.Now(), "ceph-osd@19", "first")
	expect(t, lines, "first")
	expectQuiet(t, lines)
}

func TestForbiddenSyslogExplainsThePrivilege(t *testing.T) {
	f, j := newJournal(t)
	f.forbidden = true
	r, err := j.stream(context.Background(), sdklogs.LogStreamRequest{TailLines: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, _ := io.ReadAll(r)
	if !strings.Contains(string(b), "Sys.Syslog on /nodes/ares") || strings.Count(string(b), "\n") != 1 {
		t.Errorf("forbidden stream = %q", b)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r, err = j.stream(ctx, sdklogs.LogStreamRequest{Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	lines := collect(r)
	if got := <-lines; !strings.Contains(got, "Sys.Syslog") {
		t.Errorf("follow forbidden line %q", got)
	}
	expectQuiet(t, lines) // stays open without repeating, so the SDK does not reconnect
	cancel()
	if _, open := <-lines; open {
		t.Error("line after cancel")
	}
}

func TestLocateMapsDaemonsToUnits(t *testing.T) {
	for _, tc := range []struct {
		key        string
		id         string
		data       map[string]any
		node, unit string
	}{
		{keyCephOSD, "osd.19", map[string]any{"name": "osd.19", "id": int64(19), "host": "ares"}, "ares", "ceph-osd@19"},
		{keyCephOSD, "", map[string]any{"name": "weird", "id": int64(7), "host": "apollo"}, "apollo", "ceph-osd@7"},
		{keyCephMon, "apollo", map[string]any{"name": "apollo", "host": "apollo"}, "apollo", "ceph-mon@apollo"},
		{keyCephMgr, "ares", map[string]any{"name": "ares", "host": "ares"}, "ares", "ceph-mgr@ares"},
		{keyCephMDS, "ares-ssd", map[string]any{"name": "ares-ssd", "host": "ares"}, "ares", "ceph-mds@ares-ssd"},
		{keyNode, "node/apollo", map[string]any{"id": "node/apollo", "node": "apollo"}, "apollo", ""},
		{keyNode, "node/apollo", nil, "apollo", ""},
	} {
		node, unit, err := locate(tc.key, tc.id, tc.data)
		if err != nil || node != tc.node || unit != tc.unit {
			t.Errorf("locate(%s, %v) = %q, %q, %v; want %q, %q", tc.key, tc.data, node, unit, err, tc.node, tc.unit)
		}
	}
	if _, _, err := locate(keyCephMon, "x", map[string]any{"name": "x"}); err == nil {
		t.Error("daemon without host located")
	}

	src := sourcesFor(keyCephOSD)("osd.19", body(map[string]any{"name": "osd.19", "host": "ares"}), sdklogs.LogSessionOptions{})
	if len(src) != 1 || src[0].Labels[labelNode] != "ares" || src[0].Labels[labelUnit] != "ceph-osd@19" {
		t.Errorf("sources = %+v", src)
	}
	if src := sourcesFor(keyCephOSD)("", body(map[string]any{}), sdklogs.LogSessionOptions{}); len(src) != 0 {
		t.Errorf("unlocatable resource has sources %+v", src)
	}
}
