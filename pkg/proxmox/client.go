package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	// maxResponseBytes bounds a single API response body.
	maxResponseBytes = 64 << 20
)

// ResourceType is the type filter accepted by GET /cluster/resources.
type ResourceType string

const (
	ResourceTypeVM      ResourceType = "vm" // qemu and lxc guests
	ResourceTypeNode    ResourceType = "node"
	ResourceTypeStorage ResourceType = "storage"
)

// Entry is one native API object. Raw is the unmodified object; ID is the
// field of it that identifies the object within its kind.
type Entry struct {
	ID  string
	Raw json.RawMessage
}

// ClusterResource is one /cluster/resources entry; Type routes it to a kind.
type ClusterResource struct {
	Entry
	Type string
}

// Version is the GET /version payload.
type Version struct {
	Version string `json:"version"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
}

// APIError is a non-2xx response from the Proxmox API.
type APIError struct {
	StatusCode int
	// Reason is the HTTP reason phrase; Proxmox puts its error message there.
	Reason string
	// Errors holds Proxmox's per-parameter messages from a rejected request
	// (400 parameter verification failed), keyed by parameter name.
	Errors map[string]string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("proxmox api: %d %s", e.StatusCode, e.Reason)
	if len(e.Errors) == 0 {
		return msg
	}
	keys := make([]string, 0, len(e.Errors))
	for k := range e.Errors {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + strings.TrimSpace(e.Errors[k])
	}
	return msg + " (" + strings.Join(parts, "; ") + ")"
}

// Client is a minimal, token-authenticated Proxmox VE API client.
type Client struct {
	ClusterID string
	base      *url.URL
	auth      string
	http      *http.Client
	ceph      cephRoute
	storage   guestStorageCache
}

// NewClient builds a client for cluster. The token secret lives only in the
// Authorization header and never appears in errors.
func NewClient(c Cluster) (*Client, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	base, err := c.apiBase()
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.InsecureSkipVerify} //nolint:gosec // explicit per-cluster opt-in
	if c.CAFile != "" {
		path, err := ExpandPath(c.CAFile)
		if err != nil {
			return nil, err
		}
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cluster %q: read ca_file: %w", c.ID, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("cluster %q: ca_file contains no PEM certificates", c.ID)
		}
		tlsCfg.RootCAs = pool
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg

	return &Client{
		ClusterID: c.ID,
		base:      base,
		auth:      "PVEAPIToken=" + c.TokenID + "=" + c.TokenSecret,
		http:      &http.Client{Transport: transport, Timeout: requestTimeout},
	}, nil
}

// Close releases idle connections.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// Version returns the API server version; it doubles as the auth check.
func (c *Client) Version(ctx context.Context) (Version, error) {
	var v Version
	err := c.get(ctx, "/version", nil, &v)
	return v, err
}

// ClusterResources lists cluster-wide resources of one type.
func (c *Client) ClusterResources(ctx context.Context, typ ResourceType) ([]ClusterResource, error) {
	var raw []json.RawMessage
	if err := c.get(ctx, "/cluster/resources", url.Values{"type": {string(typ)}}, &raw); err != nil {
		return nil, err
	}
	out := make([]ClusterResource, 0, len(raw))
	for _, r := range raw {
		var head struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(r, &head); err != nil {
			return nil, fmt.Errorf("decode cluster resource: %w", err)
		}
		out = append(out, ClusterResource{Entry: Entry{ID: head.ID, Raw: r}, Type: head.Type})
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, into any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, into)
}

// post sends a form-encoded POST; into, if not nil, receives the data.
func (c *Client) post(ctx context.Context, path string, form url.Values, into any) error {
	return c.do(ctx, http.MethodPost, path, nil, form, into)
}

// put sends a form-encoded PUT; into, if not nil, receives the data.
func (c *Client) put(ctx context.Context, path string, form url.Values, into any) error {
	return c.do(ctx, http.MethodPut, path, nil, form, into)
}

// del sends a DELETE with query parameters; into, if not nil, receives the data.
func (c *Client) del(ctx context.Context, path string, query url.Values, into any) error {
	return c.do(ctx, http.MethodDelete, path, query, nil, into)
}

// do sends one API request. form, when not nil, is the urlencoded body.
// into, when not nil, receives the response's data, which must be present;
// with a nil into the data is ignored (writes often answer null).
func (c *Client) do(ctx context.Context, method, path string, query, form url.Values, into any) error {
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = query.Encode()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		reason := strings.TrimSpace(strings.TrimPrefix(resp.Status, fmt.Sprint(resp.StatusCode)))
		apiErr := &APIError{StatusCode: resp.StatusCode, Reason: reason}
		var failure struct {
			Errors map[string]string `json:"errors"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&failure) == nil {
			apiErr.Errors = failure.Errors
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return apiErr
	}

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	if p, ok := into.(*paged); ok {
		p.total = envelope.Total
		into = p.into
	}
	if into == nil {
		return nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("proxmox api: %s returned no data", path)
	}
	if err := json.Unmarshal(envelope.Data, into); err != nil {
		return fmt.Errorf("decode %s data: %w", path, err)
	}
	return nil
}

// IsStatus reports whether err is an APIError with the given HTTP status.
func IsStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// paged wraps a do target for endpoints that report the full result count in
// the envelope's "total" beside a page of data (e.g. syslog).
type paged struct {
	into  any
	total int
}
