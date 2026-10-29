package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// RRDTimeframe is the window of a Proxmox RRD query. Each frame holds about
// 70 consolidated points ending now.
type RRDTimeframe string

const (
	RRDHour  RRDTimeframe = "hour"  // 1 min steps
	RRDDay   RRDTimeframe = "day"   // 30 min steps
	RRDWeek  RRDTimeframe = "week"  // 3 h steps
	RRDMonth RRDTimeframe = "month" // 12 h steps
	RRDYear  RRDTimeframe = "year"  // 1 week steps
)

// rrdFrames orders the timeframes by the span they reliably cover. The
// stored windows are a little longer (70 steps), so a requested "last hour"
// whose start drifted a few seconds still fits the hour frame.
var rrdFrames = []struct {
	frame RRDTimeframe
	span  time.Duration
}{
	{RRDHour, 70 * time.Minute},
	{RRDDay, 35 * time.Hour},
	{RRDWeek, 210 * time.Hour},
	{RRDMonth, 35 * 24 * time.Hour},
}

// TimeframeFor returns the finest timeframe whose window reaches back span.
func TimeframeFor(span time.Duration) RRDTimeframe {
	for _, f := range rrdFrames {
		if span <= f.span {
			return f.frame
		}
	}
	return RRDYear
}

// RRDPoint is one consolidated RRD sample. Values holds only the fields that
// were present and numeric; a field the node did not record (null) is absent.
type RRDPoint struct {
	Time   time.Time
	Values map[string]float64
}

// NodeRRD returns the node's RRD samples (cpu, iowait, loadavg, memused, ...).
func (c *Client) NodeRRD(ctx context.Context, node string, frame RRDTimeframe) ([]RRDPoint, error) {
	return c.rrd(ctx, "/nodes/"+url.PathEscape(node)+"/rrddata", frame)
}

// GuestRRD returns a guest's RRD samples. guestType is "qemu" or "lxc".
func (c *Client) GuestRRD(ctx context.Context, node, guestType string, vmid int, frame RRDTimeframe) ([]RRDPoint, error) {
	if guestType != "qemu" && guestType != "lxc" {
		return nil, fmt.Errorf("unknown guest type %q", guestType)
	}
	return c.rrd(ctx, fmt.Sprintf("/nodes/%s/%s/%d/rrddata", url.PathEscape(node), guestType, vmid), frame)
}

func (c *Client) rrd(ctx context.Context, path string, frame RRDTimeframe) ([]RRDPoint, error) {
	var raw []map[string]json.RawMessage
	q := url.Values{"timeframe": {string(frame)}, "cf": {"AVERAGE"}}
	if err := c.get(ctx, path, q, &raw); err != nil {
		return nil, err
	}
	points := make([]RRDPoint, 0, len(raw))
	for _, r := range raw {
		var ts float64
		if json.Unmarshal(r["time"], &ts) != nil || ts <= 0 {
			continue
		}
		values := make(map[string]float64, len(r))
		for k, v := range r {
			var f float64
			// Unmarshalling null into a float64 is a silent no-op, so check it.
			if k == "time" || string(v) == "null" || json.Unmarshal(v, &f) != nil {
				continue // null and non-numeric fields are not samples
			}
			values[k] = f
		}
		points = append(points, RRDPoint{Time: time.Unix(int64(ts), 0), Values: values})
	}
	return points, nil
}
