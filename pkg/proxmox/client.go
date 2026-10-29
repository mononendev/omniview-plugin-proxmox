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
}

func (e *APIError) Error() string {
	return fmt.Sprintf("proxmox api: %d %s", e.StatusCode, e.Reason)
}

// Client is a minimal, token-authenticated Proxmox VE API client.
type Client struct {
	ClusterID string
	base      *url.URL
	auth      string
	http      *http.Client
	ceph      cephRoute
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
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		reason := strings.TrimSpace(strings.TrimPrefix(resp.Status, fmt.Sprint(resp.StatusCode)))
		return &APIError{StatusCode: resp.StatusCode, Reason: reason}
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
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
