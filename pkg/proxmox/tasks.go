package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// taskPollInterval is how often WaitTask reads a task's status.
const taskPollInterval = time.Second

// TaskError is a Proxmox task that finished with a failure exit status.
type TaskError struct {
	UPID       string
	ExitStatus string
}

func (e *TaskError) Error() string {
	return "proxmox task failed: " + e.ExitStatus
}

// WaitTask blocks until the task UPID on node stops and returns a
// *TaskError unless it ended OK (or OK with warnings). Write endpoints of
// guests, Ceph daemons and the like answer with a UPID and run the work
// as a task; the call's own success only means the task was queued.
func (c *Client) WaitTask(ctx context.Context, node, upid string) error {
	path := "/nodes/" + url.PathEscape(node) + "/tasks/" + url.PathEscape(upid) + "/status"
	ticker := time.NewTicker(taskPollInterval)
	defer ticker.Stop()
	for {
		var status struct {
			Status     string `json:"status"`
			ExitStatus string `json:"exitstatus"`
		}
		if err := c.get(ctx, path, nil, &status); err != nil {
			return fmt.Errorf("task %s status: %w", upid, err)
		}
		if status.Status == "stopped" {
			if status.ExitStatus == "OK" || strings.HasPrefix(status.ExitStatus, "WARNINGS") {
				return nil
			}
			return &TaskError{UPID: upid, ExitStatus: status.ExitStatus}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
