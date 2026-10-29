package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestSyslogMapsQueryAndReadsTotal(t *testing.T) {
	var got url.Values
	body := `{"total":120,"data":[{"n":101,"t":"Oct 29 00:15:15 ares ceph-osd[1]: a"},{"n":102,"t":"Oct 29 00:15:16 ares ceph-osd[1]: b"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/ares/syslog" {
			t.Errorf("path %s", r.URL.Path)
		}
		got = r.URL.Query()
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ny := time.FixedZone("EDT", -4*3600)
	since := time.Date(2026, 10, 29, 4, 0, 30, 500, time.UTC) // 00:00:30 in the node's zone

	page, err := client.Syslog(context.Background(), "ares", SyslogQuery{Service: "ceph-osd@19", Since: since, Start: 100, Limit: 2}, ny)
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{"service": {"ceph-osd@19"}, "since": {"2026-10-29 00:00:30"}, "start": {"100"}, "limit": {"2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query = %v, want %v", got, want)
	}
	if page.Total != 120 || len(page.Lines) != 2 || page.Lines[1] != "Oct 29 00:15:16 ares ceph-osd[1]: b" {
		t.Errorf("page = %+v", page)
	}

	if _, err := client.Syslog(context.Background(), "ares", SyslogQuery{}, ny); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("empty query sent %v", got)
	}

	for _, placeholder := range []string{"no content", "-- No entries --"} {
		body = `{"total":1,"data":[{"n":1,"t":"` + placeholder + `"}]}`
		page, err = client.Syslog(context.Background(), "ares", SyslogQuery{}, ny)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 0 || len(page.Lines) != 0 {
			t.Errorf("placeholder %q read as %+v", placeholder, page)
		}
	}
}

func TestNodeTimeUsesTheNodeZone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"time":1793247337,"localtime":1793232937,"timezone":"No/Such_Zone"}}`))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	nt, err := client.NodeTime(context.Background(), "ares")
	if err != nil {
		t.Fatal(err)
	}
	if !nt.Now.Equal(time.Unix(1793247337, 0)) {
		t.Errorf("now = %v", nt.Now)
	}
	if _, off := nt.Now.Zone(); off != -4*3600 {
		t.Errorf("unknown zone fell back to offset %d, want the reported -14400", off)
	}
}

func TestParseSyslogLine(t *testing.T) {
	ny := time.FixedZone("EDT", -4*3600)
	now := time.Date(2026, 10, 29, 0, 20, 0, 0, ny)
	jan := time.Date(2027, 1, 2, 0, 0, 0, 0, ny)
	for _, tc := range []struct {
		line string
		now  time.Time
		want time.Time
		rest string
		ok   bool
	}{
		{"Oct 29 00:15:15 ares ceph-mgr[3673]: GET /metrics", now, time.Date(2026, 10, 29, 0, 15, 15, 0, ny), "ares ceph-mgr[3673]: GET /metrics", true},
		{"Oct  9 08:01:02 ares kernel: x", now, time.Date(2026, 10, 9, 8, 1, 2, 0, ny), "ares kernel: x", true},
		{"Oct 09 08:01:02 ares kernel: x", now, time.Date(2026, 10, 9, 8, 1, 2, 0, ny), "ares kernel: x", true},
		{"Dec 31 23:59:59 ares cron: y", jan, time.Date(2026, 12, 31, 23, 59, 59, 0, ny), "ares cron: y", true},
		{"2026-10-29T00:15:15-0400 ares sshd: z", now, time.Date(2026, 10, 29, 4, 15, 15, 0, time.UTC), "ares sshd: z", true},
		{"2026-10-29T00:15:15.250000-0400 ares sshd: z", now, time.Date(2026, 10, 29, 4, 15, 15, 250000000, time.UTC), "ares sshd: z", true},
		{"-- Boot 0123abcd --", now, time.Time{}, "-- Boot 0123abcd --", false},
		{"    continuation", now, time.Time{}, "    continuation", false},
	} {
		ts, rest, ok := ParseSyslogLine(tc.line, ny, tc.now)
		if ok != tc.ok || !ts.Equal(tc.want) || rest != tc.rest {
			t.Errorf("ParseSyslogLine(%q) = %v, %q, %v; want %v, %q, %v", tc.line, ts, rest, ok, tc.want, tc.rest, tc.ok)
		}
	}
}
