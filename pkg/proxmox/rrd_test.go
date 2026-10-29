package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTimeframeForPicksTheFinestCoveringWindow(t *testing.T) {
	for _, tc := range []struct {
		span time.Duration
		want RRDTimeframe
	}{
		{0, RRDHour},
		{15 * time.Minute, RRDHour},
		{time.Hour + 30*time.Second, RRDHour}, // "last hour" whose start drifted
		{2 * time.Hour, RRDDay},
		{24 * time.Hour, RRDDay},
		{3 * 24 * time.Hour, RRDWeek},
		{7 * 24 * time.Hour, RRDWeek},
		{30 * 24 * time.Hour, RRDMonth},
		{90 * 24 * time.Hour, RRDYear},
	} {
		if got := TimeframeFor(tc.span); got != tc.want {
			t.Errorf("TimeframeFor(%v) = %s, want %s", tc.span, got, tc.want)
		}
	}
}

func TestRRDRequestsFrameAndSkipsNullSamples(t *testing.T) {
	var got []*url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL)
		_, _ = w.Write([]byte(`{"data":[` +
			`{"time":1000,"cpu":0.25,"mem":1024,"netin":null},` +
			`{"time":1060},` +
			`{"time":null,"cpu":0.5}]}`))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}

	points, err := client.GuestRRD(context.Background(), "pve1", "qemu", 100, RRDDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || strings.TrimPrefix(got[0].Path, "/api2/json") != "/nodes/pve1/qemu/100/rrddata" ||
		got[0].Query().Get("timeframe") != "day" || got[0].Query().Get("cf") != "AVERAGE" {
		t.Fatalf("requested %v", got)
	}
	if len(points) != 2 {
		t.Fatalf("points = %+v, want the two with a time", points)
	}
	first := points[0]
	if !first.Time.Equal(time.Unix(1000, 0)) || first.Values["cpu"] != 0.25 || first.Values["mem"] != 1024 {
		t.Errorf("first point = %+v", first)
	}
	if _, ok := first.Values["netin"]; ok {
		t.Errorf("null netin decoded as a sample: %+v", first)
	}
	if len(points[1].Values) != 0 {
		t.Errorf("empty sample has values %+v", points[1].Values)
	}

	if _, err := client.NodeRRD(context.Background(), "pve1", RRDHour); err != nil {
		t.Fatal(err)
	}
	if p := strings.TrimPrefix(got[1].Path, "/api2/json"); p != "/nodes/pve1/rrddata" || got[1].Query().Get("timeframe") != "hour" {
		t.Errorf("node rrd requested %s?%s", p, got[1].RawQuery)
	}
	if _, err := client.GuestRRD(context.Background(), "pve1", "storage", 1, RRDHour); err == nil {
		t.Error("unknown guest type accepted")
	}
}
