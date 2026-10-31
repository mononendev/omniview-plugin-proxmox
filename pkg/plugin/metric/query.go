package metric

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdkmetric "github.com/omniviewdev/plugin-sdk/pkg/v1/metric"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// queryTimeout bounds one query, which may take two API calls.
const queryTimeout = 30 * time.Second

func query(qctx *sdkmetric.QueryContext, req sdkmetric.QueryRequest) (*sdkmetric.QueryResponse, error) {
	k, ok := kinds[req.ResourceKey]
	if !ok {
		return failure("no Proxmox metrics for resource %s", req.ResourceKey), nil
	}
	if qctx == nil || qctx.Connection == nil {
		return failure("no connection in metric query"), nil
	}
	client, err := proxmox.ClientForConnection(qctx.Connection)
	if err != nil {
		return failure("%v", err), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	return run(ctx, client, k, req, time.Now()), nil
}

// run answers req for one resource of kind k; now anchors the RRD windows.
func run(ctx context.Context, client *proxmox.Client, k kind, req sdkmetric.QueryRequest, now time.Time) *sdkmetric.QueryResponse {
	wanted, err := selectSeries(k.series, req.MetricIDs)
	if err != nil {
		return failure("%v", err)
	}

	var start, end time.Time
	frame := proxmox.RRDHour
	switch req.Shape {
	case sdkmetric.ShapeCurrent:
	case sdkmetric.ShapeTimeseries:
		start, end = req.StartTime, req.EndTime
		if end.IsZero() || end.After(now) {
			end = now
		}
		if start.IsZero() {
			start = end.Add(-time.Hour)
		}
		frame = proxmox.TimeframeFor(now.Sub(start))
	default:
		return failure("metric shape %d is not supported", req.Shape)
	}

	t, err := resolveTarget(ctx, client, k, req)
	if err != nil {
		return failure("%v", err)
	}
	var points []proxmox.RRDPoint
	if k.guestType == "" {
		points, err = client.NodeRRD(ctx, t.node, frame)
	} else {
		points, err = client.GuestRRD(ctx, t.node, k.guestType, t.vmid, frame)
	}
	if err != nil {
		return failure("%s", describeError(err, k, t))
	}

	results := make([]sdkmetric.MetricResult, 0, len(wanted))
	for _, s := range wanted {
		if req.Shape == sdkmetric.ShapeCurrent {
			if cv := latest(points, s); cv != nil {
				results = append(results, sdkmetric.MetricResult{CurrentValue: cv})
			}
			continue
		}
		results = append(results, sdkmetric.MetricResult{TimeSeries: timeseries(points, s, start, end)})
	}
	return &sdkmetric.QueryResponse{Success: true, Results: results}
}

// selectSeries returns the requested series in request order; no IDs means
// all of them. Unknown IDs are an error only when nothing else matched.
func selectSeries(all []series, ids []string) ([]series, error) {
	if len(ids) == 0 {
		return all, nil
	}
	out := make([]series, 0, len(ids))
	for _, id := range ids {
		for _, s := range all {
			if s.desc.ID == id {
				out = append(out, s)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("unknown metrics %s", strings.Join(ids, ", "))
	}
	return out, nil
}

func timeseries(points []proxmox.RRDPoint, s series, start, end time.Time) *sdkmetric.TimeSeries {
	ts := &sdkmetric.TimeSeries{MetricID: s.desc.ID, DataPoints: make([]sdkmetric.DataPoint, 0, len(points))}
	for _, p := range points {
		if p.Time.Before(start) || p.Time.After(end) {
			continue
		}
		if v, ok := p.Values[s.field]; ok {
			ts.DataPoints = append(ts.DataPoints, sdkmetric.DataPoint{Timestamp: p.Time, Value: v * s.scale})
		}
	}
	return ts
}

// latest is the newest recorded sample of s, or nil if there is none (e.g.
// a guest that has not run during the window).
func latest(points []proxmox.RRDPoint, s series) *sdkmetric.CurrentValue {
	for i := len(points) - 1; i >= 0; i-- {
		if v, ok := points[i].Values[s.field]; ok {
			return &sdkmetric.CurrentValue{MetricID: s.desc.ID, Value: v * s.scale, Timestamp: points[i].Time}
		}
	}
	return nil
}

// target is the node, and for guests the vmid, a query reads.
type target struct {
	node string
	vmid int
}

// resolveTarget reads the node and vmid from the resource object (a
// /cluster/resources entry), falling back to the resource ID ("node/apollo",
// "qemu/100") and, for guests without a node, a /cluster/resources lookup.
func resolveTarget(ctx context.Context, client *proxmox.Client, k kind, req sdkmetric.QueryRequest) (target, error) {
	data := proxmox.ResourceData(req.ResourceData)
	id := stringField(data, "id")
	if id == "" {
		id = req.ResourceID
	}

	if k.guestType == "" {
		node := stringField(data, "node")
		if node == "" {
			node = strings.TrimPrefix(id, "node/")
		}
		if node == "" {
			return target{}, fmt.Errorf("cannot tell which node to query: no node in the resource")
		}
		return target{node: node}, nil
	}

	vmid, ok := intField(data, "vmid")
	if !ok {
		n, err := strconv.Atoi(strings.TrimPrefix(id, k.guestType+"/"))
		if err != nil {
			return target{}, fmt.Errorf("cannot tell which guest to query from resource %q", id)
		}
		vmid = n
	}
	t := target{node: stringField(data, "node"), vmid: vmid}
	if t.node != "" {
		return t, nil
	}

	guests, err := client.ClusterResources(ctx, proxmox.ResourceTypeVM)
	if err != nil {
		return target{}, fmt.Errorf("locate %s/%d: %w", k.guestType, vmid, err)
	}
	want := fmt.Sprintf("%s/%d", k.guestType, vmid)
	for _, g := range guests {
		if g.ID != want {
			continue
		}
		var entry struct {
			Node string `json:"node"`
		}
		if err := json.Unmarshal(g.Raw, &entry); err != nil {
			return target{}, fmt.Errorf("decode %s: %w", want, err)
		}
		t.node = entry.Node
		return t, nil
	}
	return target{}, fmt.Errorf("guest %s not found in the cluster", want)
}

// describeError names the privilege a 403 is missing.
func describeError(err error, k kind, t target) string {
	if proxmox.IsStatus(err, http.StatusForbidden) {
		if k.guestType == "" {
			return fmt.Sprintf("the API token needs Sys.Audit on /nodes/%s to read metrics (%v)", t.node, err)
		}
		return fmt.Sprintf("the API token needs VM.Audit on /vms/%d to read metrics (%v)", t.vmid, err)
	}
	return err.Error()
}

func failure(format string, args ...any) *sdkmetric.QueryResponse {
	return &sdkmetric.QueryResponse{Success: false, Error: fmt.Sprintf(format, args...)}
}

func stringField(data map[string]any, key string) string {
	s, _ := data[key].(string)
	return s
}

// intField reads an integer resource field: int64 or uint64 from the
// resource body, or a decimal string.
func intField(data map[string]any, key string) (int, bool) {
	switch v := data[key].(type) {
	case int64:
		return int(v), true
	case uint64:
		return int(v), v <= math.MaxInt
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	}
	return 0, false
}
