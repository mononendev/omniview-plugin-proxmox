// Package proxmox holds the cluster configuration and the minimal Proxmox VE
// API client used by the plugin.
package proxmox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Cluster is one configured Proxmox VE API endpoint: a cluster or a
// standalone node. It becomes one Omniview connection.
type Cluster struct {
	// ID is the stable connection identifier. It must be unique across all
	// configured files.
	ID string `yaml:"id"`
	// Name is the display name. Defaults to ID.
	Name string `yaml:"name"`
	// URL is the API base, e.g. https://pve1.lan:8006. It must not include the
	// /api2/json path.
	URL string `yaml:"url"`
	// TokenID is the API token identifier, USER@REALM!TOKENNAME.
	TokenID string `yaml:"token_id"`
	// TokenSecret is the API token UUID secret.
	TokenSecret string `yaml:"token_secret"`
	// CAFile optionally pins a PEM bundle for the server certificate.
	CAFile string `yaml:"ca_file"`
	// InsecureSkipVerify disables TLS verification. Prefer CAFile.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

type configFile struct {
	Clusters []Cluster `yaml:"clusters"`
}

// ExpandPath resolves a leading ~ to the user's home directory.
func ExpandPath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

// LoadFile reads and validates the clusters in one config file. A missing
// file yields no clusters and an error wrapping fs.ErrNotExist.
func LoadFile(path string) ([]Cluster, error) {
	resolved, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg configFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]struct{}, len(cfg.Clusters))
	for i := range cfg.Clusters {
		c := &cfg.Clusters[i]
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("%s: clusters[%d]: %w", path, i, err)
		}
		if _, dup := seen[c.ID]; dup {
			return nil, fmt.Errorf("%s: duplicate cluster id %q", path, c.ID)
		}
		seen[c.ID] = struct{}{}
		if c.Name == "" {
			c.Name = c.ID
		}
	}
	return cfg.Clusters, nil
}

// FindCluster loads path and returns the cluster with the given ID.
func FindCluster(path, id string) (Cluster, error) {
	clusters, err := LoadFile(path)
	if err != nil {
		return Cluster{}, err
	}
	for _, c := range clusters {
		if c.ID == id {
			return c, nil
		}
	}
	return Cluster{}, fmt.Errorf("cluster %q is no longer configured in %s", id, path)
}

func (c Cluster) validate() error {
	if c.ID == "" {
		return errors.New("id is required")
	}
	if c.TokenID == "" || !strings.Contains(c.TokenID, "!") {
		return fmt.Errorf("cluster %q: token_id must be USER@REALM!TOKENNAME", c.ID)
	}
	if c.TokenSecret == "" {
		return fmt.Errorf("cluster %q: token_secret is required", c.ID)
	}
	if _, err := c.apiBase(); err != nil {
		return fmt.Errorf("cluster %q: %w", c.ID, err)
	}
	return nil
}

// apiBase returns the /api2/json base URL for the cluster.
func (c Cluster) apiBase() (*url.URL, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("url %q must use https", c.URL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("url %q has no host", c.URL)
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("url %q must not include a path; use the server root such as https://pve.lan:8006", c.URL)
	}
	if u.RawQuery != "" || u.User != nil {
		return nil, fmt.Errorf("url %q must not include credentials or a query", c.URL)
	}
	u.Path = "/api2/json"
	return u, nil
}
