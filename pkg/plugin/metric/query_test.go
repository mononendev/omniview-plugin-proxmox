package metric

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"
	sdkmetric "github.com/omniviewdev/plugin-sdk/pkg/v1/metric"

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

// now anchors every test; the fake RRD holds one sample per minute for the
// last 70 minutes, with cpu = minute/100 and gaps (nulls) at minutes 3 and 1.
var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

type fakeRRD struct {
	mu       sync.Mutex
	requests []string // "path?timeframe"
	status   int
}

func (f *fakeRRD) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	f.requests = append(f.requests, path+"?"+r.URL.Query().Get("timeframe"))
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if path == "/cluster/resources" {
		_, _ = w.Write([]byte(`{"data":[{"id":"lxc/1000","type":"lxc","vmid":1000,"node":"ares"},{"id":"qemu/101","type":"qemu","vmid":101,"node":"ares"}]}`))
		return
	}
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for m := 70; m >= 1; m-- {
		ts := now.Add(-time.Duration(m) * time.Minute).Unix()
		if m < 70 {
			b.WriteByte(',')
		}
		if m == 3 || m == 1 {
			fmt.Fprintf(&b, `{"time":%d,"cpu":null,"mem":null}`, ts)
			continue
		}
		fmt.Fprintf(&b, `{"time":%d,"cpu":%g,"iowait":0.01,"memused":%d,"memtotal":4096,"mem":%d,"maxmem":8192,`+
			`"netin":10,"netout":20,"diskread":30,"diskwrite":40,"loadavg":1.5,"rootused":100,"roottotal":200}`,
			ts, float64(m)/100, m, m)
	}
	// The newest sample has not been consolidated yet.
	fmt.Fprintf(&b, `,{"time":%d}]}`, now.Unix())
	_, _ = w.Write([]byte(b.String()))
}

func (f *fakeRRD) lastRequest() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func newFake(t *testing.T) (*fakeRRD, *proxmox.Client) {
	t.Helper()
	f := &fakeRRD{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	return f, client
}

func mustSucceed(t *testing.T, resp *sdkmetric.QueryResponse) []sdkmetric.MetricResult {
	t.Helper()
	if !resp.Success {
		t.Fatalf("query failed: %s", resp.Error)
	}
	return resp.Results
}

func TestTimeseriesScalesCPUAndFiltersToTheRange(t *testing.T) {
	f, client := newFake(t)
	resp := run(context.Background(), client, kinds[keyNode], sdkmetric.QueryRequest{
		ResourceKey:  keyNode,
		ResourceData: body(map[string]any{"id": "node/ares", "node": "ares"}),
		MetricIDs:    []string{"cpu", "memory"},
		Shape:        sdkmetric.ShapeTimeseries,
		StartTime:    now.Add(-10 * time.Minute),
		EndTime:      now.Add(-2 * time.Minute),
	}, now)
	results := mustSucceed(t, resp)

	if got := f.lastRequest(); got != "/nodes/ares/rrddata?hour" {
		t.Errorf("requested %s", got)
	}
	if len(results) != 2 || results[0].TimeSeries.MetricID != "cpu" || results[1].TimeSeries.MetricID != "memory" {
		t.Fatalf("results = %+v", results)
	}
	cpu := results[0].TimeSeries.DataPoints
	// Minutes 10..2 are in range; minute 3 has no sample.
	if len(cpu) != 8 {
		t.Fatalf("cpu points = %+v", cpu)
	}
	if first := cpu[0]; !first.Timestamp.Equal(now.Add(-10*time.Minute)) || first.Value != 10 {
		t.Errorf("first cpu point = %+v, want 10%% at -10m", first)
	}
	if last := cpu[len(cpu)-1]; !last.Timestamp.Equal(now.Add(-2*time.Minute)) || last.Value != 2 {
		t.Errorf("last cpu point = %+v, want 2%% at -2m", last)
	}
	if mem := results[1].TimeSeries.DataPoints; mem[0].Value != 10 {
		t.Errorf("node memory read %v, want memused", mem[0].Value)
	}
}

func TestTimeseriesTimeframeFollowsTheRequestedRange(t *testing.T) {
	f, client := newFake(t)
	for _, tc := range []struct {
		start time.Time
		want  string
	}{
		{time.Time{}, "hour"},
		{now.Add(-time.Hour), "hour"},
		{now.Add(-6 * time.Hour), "day"},
		{now.Add(-24 * time.Hour), "day"},
		{now.Add(-7 * 24 * time.Hour), "week"},
		{now.Add(-30 * 24 * time.Hour), "month"},
		{now.Add(-365 * 24 * time.Hour), "year"},
	} {
		mustSucceed(t, run(context.Background(), client, kinds[keyVM], sdkmetric.QueryRequest{
			ResourceKey:  keyVM,
			ResourceData: body(map[string]any{"id": "qemu/101", "node": "ares", "vmid": 101}),
			MetricIDs:    []string{"cpu"},
			Shape:        sdkmetric.ShapeTimeseries,
			StartTime:    tc.start,
			EndTime:      now,
		}, now))
		if got := f.lastRequest(); got != "/nodes/ares/qemu/101/rrddata?"+tc.want {
			t.Errorf("start %v requested %s, want timeframe %s", now.Sub(tc.start), got, tc.want)
		}
	}
}

func TestCurrentIsTheLatestRecordedSample(t *testing.T) {
	_, client := newFake(t)
	results := mustSucceed(t, run(context.Background(), client, kinds[keyContainer], sdkmetric.QueryRequest{
		ResourceKey:  keyContainer,
		ResourceData: body(map[string]any{"id": "lxc/1000", "node": "ares", "vmid": 1000}),
		Shape:        sdkmetric.ShapeCurrent,
	}, now))

	byID := map[string]*sdkmetric.CurrentValue{}
	for _, r := range results {
		if r.CurrentValue == nil {
			t.Fatalf("current query returned %+v", r)
		}
		byID[r.CurrentValue.MetricID] = r.CurrentValue
	}
	if len(byID) != len(guestSeries) {
		t.Errorf("metrics = %v, want all guest metrics", byID)
	}
	// The newest sample is empty and minute 3 is null, so cpu comes from minute 2.
	if cpu := byID["cpu"]; cpu.Value != 2 || !cpu.Timestamp.Equal(now.Add(-2*time.Minute)) {
		t.Errorf("cpu = %+v, want 2%% at -2m", cpu)
	}
	if mem := byID["memory"]; mem.Value != 2 {
		t.Errorf("memory = %+v, want the guest's mem", mem)
	}
	for id, want := range map[string]float64{"memory_total": 8192, "net_in": 10, "net_out": 20, "disk_read": 30, "disk_write": 40} {
		if byID[id] == nil || byID[id].Value != want {
			t.Errorf("%s = %+v, want %v", id, byID[id], want)
		}
	}
}

func TestNodeOnlyMetricsAndUnits(t *testing.T) {
	_, client := newFake(t)
	results := mustSucceed(t, run(context.Background(), client, kinds[keyNode], sdkmetric.QueryRequest{
		ResourceKey: keyNode,
		ResourceID:  "node/ares",
		MetricIDs:   []string{"iowait", "load", "rootfs", "rootfs_total", "memory_total", "disk_read"},
		Shape:       sdkmetric.ShapeCurrent,
	}, now))
	got := map[string]float64{}
	for _, r := range results {
		got[r.CurrentValue.MetricID] = r.CurrentValue.Value
	}
	want := map[string]float64{"iowait": 1, "load": 1.5, "rootfs": 100, "rootfs_total": 200, "memory_total": 4096}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("node current = %v, want %v (disk_read is guest-only)", got, want)
	}
}

func TestGuestNodeIsLookedUpWhenTheObjectLacksIt(t *testing.T) {
	f, client := newFake(t)
	mustSucceed(t, run(context.Background(), client, kinds[keyContainer], sdkmetric.QueryRequest{
		ResourceKey: keyContainer,
		ResourceID:  "lxc/1000",
		Shape:       sdkmetric.ShapeCurrent,
	}, now))
	if got := f.lastRequest(); got != "/nodes/ares/lxc/1000/rrddata?hour" {
		t.Errorf("requested %s", got)
	}

	resp := run(context.Background(), client, kinds[keyVM], sdkmetric.QueryRequest{ResourceKey: keyVM, ResourceID: "qemu/999"}, now)
	if resp.Success || !strings.Contains(resp.Error, "qemu/999 not found") {
		t.Errorf("missing guest: %+v", resp)
	}
}

func TestForbiddenNamesThePrivilege(t *testing.T) {
	f, client := newFake(t)
	f.status = http.StatusForbidden
	resp := run(context.Background(), client, kinds[keyVM], sdkmetric.QueryRequest{
		ResourceKey:  keyVM,
		ResourceData: body(map[string]any{"node": "ares", "vmid": 101}),
	}, now)
	if resp.Success || !strings.Contains(resp.Error, "VM.Audit on /vms/101") {
		t.Errorf("forbidden: %+v", resp)
	}
	resp = run(context.Background(), client, kinds[keyNode], sdkmetric.QueryRequest{ResourceKey: keyNode, ResourceID: "node/ares"}, now)
	if resp.Success || !strings.Contains(resp.Error, "Sys.Audit on /nodes/ares") {
		t.Errorf("forbidden: %+v", resp)
	}
}

func TestUnknownMetricsAndShapes(t *testing.T) {
	_, client := newFake(t)
	resp := run(context.Background(), client, kinds[keyVM], sdkmetric.QueryRequest{
		ResourceID: "qemu/101", MetricIDs: []string{"load"},
	}, now)
	if resp.Success {
		t.Error("node-only metric served for a VM")
	}
	resp = run(context.Background(), client, kinds[keyVM], sdkmetric.QueryRequest{
		ResourceID: "qemu/101", Shape: sdkmetric.ShapeAggregate,
	}, now)
	if resp.Success {
		t.Error("aggregate shape served")
	}
	resp, err := query(&sdkmetric.QueryContext{}, sdkmetric.QueryRequest{ResourceKey: keyVM})
	if err != nil || resp.Success || !strings.Contains(resp.Error, "no connection") {
		t.Errorf("query without connection = %+v, %v", resp, err)
	}
}

func TestRegisteredDescriptors(t *testing.T) {
	ids := func(ss []series) string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = s.desc.ID
		}
		return strings.Join(out, ",")
	}
	if got := ids(nodeSeries); got != "cpu,iowait,memory,memory_total,net_in,net_out,load,rootfs,rootfs_total" {
		t.Errorf("node metrics %s", got)
	}
	if got := ids(guestSeries); got != "cpu,memory,memory_total,net_in,net_out,disk_read,disk_write" {
		t.Errorf("guest metrics %s", got)
	}
	units := map[string]sdkmetric.MetricUnit{
		"cpu": sdkmetric.UnitPercentage, "iowait": sdkmetric.UnitPercentage, "memory": sdkmetric.UnitBytes,
		"memory_total": sdkmetric.UnitBytes, "net_in": sdkmetric.UnitBytesPerSec, "net_out": sdkmetric.UnitBytesPerSec,
		"disk_read": sdkmetric.UnitBytesPerSec, "disk_write": sdkmetric.UnitBytesPerSec, "load": sdkmetric.UnitCount,
		"rootfs": sdkmetric.UnitBytes, "rootfs_total": sdkmetric.UnitBytes,
	}
	for _, s := range append(append([]series{}, nodeSeries...), guestSeries...) {
		if s.desc.Unit != units[s.desc.ID] {
			t.Errorf("%s unit %d, want %d", s.desc.ID, s.desc.Unit, units[s.desc.ID])
		}
		if len(s.desc.SupportedShapes) != 2 || s.desc.ChartGroup == "" {
			t.Errorf("%s descriptor %+v", s.desc.ID, s.desc)
		}
	}
}
