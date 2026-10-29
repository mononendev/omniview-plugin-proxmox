package proxmox

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SyslogTimeLayout is the node-local wall-clock format the syslog API's
// since/until parameters take.
const SyslogTimeLayout = "2006-01-02 15:04:05"

// SyslogQuery selects lines of a node's journal (journalctl output).
type SyslogQuery struct {
	// Service restricts the journal to one systemd unit; empty is everything.
	Service string
	// Since, if not zero, drops lines before it. It is sent at second
	// resolution in the node's time zone, loc.
	Since time.Time
	// Start skips that many lines of the result; Limit caps the page.
	Start int
	Limit int
}

// SyslogPage is one page of journal lines and the full result's line count.
type SyslogPage struct {
	Lines []string
	Total int
}

// Syslog reads a page of the node's journal. It needs Sys.Syslog on
// /nodes/{node}. loc is the node's time zone, used to express q.Since.
func (c *Client) Syslog(ctx context.Context, node string, q SyslogQuery, loc *time.Location) (SyslogPage, error) {
	params := url.Values{}
	if q.Service != "" {
		params.Set("service", q.Service)
	}
	if !q.Since.IsZero() {
		params.Set("since", q.Since.In(loc).Format(SyslogTimeLayout))
	}
	if q.Start > 0 {
		params.Set("start", strconv.Itoa(q.Start))
	}
	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
	}

	var lines []struct {
		N int    `json:"n"`
		T string `json:"t"`
	}
	page := &paged{into: &lines}
	if err := c.get(ctx, "/nodes/"+url.PathEscape(node)+"/syslog", params, page); err != nil {
		return SyslogPage{}, err
	}

	// An empty journal comes back as a single placeholder line: Proxmox adds
	// "no content" for its UI, journalctl itself may print "-- No entries --".
	if page.total <= 1 && len(lines) == 1 && (lines[0].T == "no content" || lines[0].T == "-- No entries --") {
		return SyslogPage{Lines: []string{}}, nil
	}
	out := SyslogPage{Lines: make([]string, len(lines)), Total: page.total}
	for i, l := range lines {
		out.Lines[i] = l.T
	}
	return out, nil
}

// NodeTime is a node's clock and time zone.
type NodeTime struct {
	Now      time.Time
	Location *time.Location
}

// NodeTime reads the node's clock and time zone (GET /nodes/{node}/time).
func (c *Client) NodeTime(ctx context.Context, node string) (NodeTime, error) {
	var t struct {
		Time      int64  `json:"time"`
		LocalTime int64  `json:"localtime"`
		Timezone  string `json:"timezone"`
	}
	if err := c.get(ctx, "/nodes/"+url.PathEscape(node)+"/time", nil, &t); err != nil {
		return NodeTime{}, err
	}
	loc, err := time.LoadLocation(t.Timezone)
	if err != nil || t.Timezone == "" {
		// No tzdata for the zone here: the current offset is the best
		// approximation (it misses DST transitions).
		loc = time.FixedZone(t.Timezone, int(t.LocalTime-t.Time))
	}
	return NodeTime{Now: time.Unix(t.Time, 0).In(loc), Location: loc}, nil
}

// ParseSyslogLine splits a journalctl line into its timestamp and the rest.
// It understands the short format ("Oct 29 03:55:36 host unit[pid]: msg"),
// whose wall clock is in the node's zone loc and has no year (inferred from
// now), and the short-iso/short-iso-precise formats. ok is false for lines
// without a timestamp, such as "-- Boot ... --" markers.
func ParseSyslogLine(line string, loc *time.Location, now time.Time) (ts time.Time, rest string, ok bool) {
	if len(line) >= 15 && line[3] == ' ' && line[6] == ' ' && line[9] == ':' && line[12] == ':' {
		t, err := time.ParseInLocation("Jan _2 15:04:05", line[:15], loc)
		if err == nil {
			now = now.In(loc)
			t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
			// A line "from the future" is from last year (e.g. Dec in Jan).
			if t.After(now.Add(48 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			return t, strings.TrimPrefix(line[15:], " "), true
		}
	}
	field, after, _ := strings.Cut(line, " ")
	if len(field) >= 20 && field[4] == '-' && field[10] == 'T' {
		for _, layout := range []string{"2006-01-02T15:04:05.999999999-0700", time.RFC3339Nano} {
			if t, err := time.Parse(layout, field); err == nil {
				return t, after, true
			}
		}
	}
	return time.Time{}, line, false
}
