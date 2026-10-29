package resource

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sort"

	"github.com/omniviewdev/plugin-sdk/pkg/types"
	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// SettingConfigFiles is the plugin setting listing cluster config files.
const SettingConfigFiles = "config_files"

// DefaultConfigFile is the initial value of SettingConfigFiles.
const DefaultConfigFile = "~/.config/omniview-proxmox/clusters.yaml"

// Connection.Data keys. The token secret is deliberately absent: CreateClient
// re-reads it from the config file so it never crosses the plugin boundary.
const (
	dataURL        = "url"
	dataTokenID    = "token_id"
	dataInsecure   = "insecure_skip_verify"
	dataConfigFile = "config_file"
)

var _ sdkresource.ConnectionProvider[proxmox.Client] = (*connectionProvider)(nil)

type connectionProvider struct{}

func (p *connectionProvider) LoadConnections(ctx context.Context) ([]types.Connection, error) {
	session := sdkresource.SessionFromContext(ctx)
	if session == nil || session.PluginConfig == nil {
		return nil, errors.New("no plugin settings in context")
	}
	files, err := session.PluginConfig.GetStringSlice(SettingConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("read %s setting: %w", SettingConfigFiles, err)
	}
	return loadConnections(files)
}

func loadConnections(files []string) ([]types.Connection, error) {
	connections := make([]types.Connection, 0, len(files))
	owner := make(map[string]string)
	for _, file := range files {
		clusters, err := proxmox.LoadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, c := range clusters {
			if prev, dup := owner[c.ID]; dup {
				return nil, fmt.Errorf("cluster id %q is defined in both %s and %s", c.ID, prev, file)
			}
			owner[c.ID] = file
			conn, err := types.NewConnection(types.ConnectionOpts{
				ID:   c.ID,
				Name: c.Name,
				Data: map[string]any{
					dataURL:        c.URL,
					dataTokenID:    c.TokenID,
					dataInsecure:   c.InsecureSkipVerify,
					dataConfigFile: file,
				},
			})
			if err != nil {
				return nil, err
			}
			connections = append(connections, *conn)
		}
	}
	sort.Slice(connections, func(i, j int) bool { return connections[i].Name < connections[j].Name })
	return connections, nil
}

func (p *connectionProvider) CreateClient(ctx context.Context) (*proxmox.Client, error) {
	session := sdkresource.SessionFromContext(ctx)
	if session == nil || session.Connection == nil {
		return nil, errors.New("no connection in context")
	}
	return newClient(session.Connection)
}

// newClient builds a client for conn, reading its token secret from the
// connection's config file.
func newClient(conn *types.Connection) (*proxmox.Client, error) {
	file, _ := conn.Data[dataConfigFile].(string)
	if file == "" {
		return nil, fmt.Errorf("connection %q has no %s", conn.ID, dataConfigFile)
	}
	cluster, err := proxmox.FindCluster(file, conn.ID)
	if err != nil {
		return nil, err
	}
	return proxmox.NewClient(cluster)
}

func (p *connectionProvider) DestroyClient(_ context.Context, client *proxmox.Client) error {
	if client != nil {
		client.Close()
	}
	return nil
}

func (p *connectionProvider) CheckConnection(
	ctx context.Context,
	conn *types.Connection,
	client *proxmox.Client,
) (types.ConnectionStatus, error) {
	status := types.ConnectionStatus{Connection: conn}
	if client == nil {
		status.Status = types.ConnectionStatusError
		status.Error = "client is required"
		return status, nil
	}
	if _, err := client.Version(ctx); err != nil {
		status.Status, status.Error, status.Details = classifyConnectionError(err)
		return status, nil
	}
	status.Status = types.ConnectionStatusConnected
	status.Details = "Connected to the Proxmox VE API"
	return status, nil
}

// GetNamespaces returns none: Proxmox nodes are native scope, not namespaces.
func (p *connectionProvider) GetNamespaces(context.Context, *proxmox.Client) ([]string, error) {
	return []string{}, nil
}

func classifyConnectionError(err error) (types.ConnectionStatusCode, string, string) {
	var (
		dnsErr      *net.DNSError
		netErr      net.Error
		unknownCA   x509.UnknownAuthorityError
		hostname    x509.HostnameError
		certInvalid x509.CertificateInvalidError
	)
	switch {
	case proxmox.IsStatus(err, http.StatusUnauthorized):
		return types.ConnectionStatusUnauthorized, "Unauthorized",
			"The API token was rejected. Check token_id and token_secret."
	case proxmox.IsStatus(err, http.StatusForbidden):
		return types.ConnectionStatusForbidden, "Forbidden",
			"The API token lacks permission. Grant it at least PVEAuditor on /."
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &certInvalid):
		return types.ConnectionStatusError, "Certificate rejected",
			"The server certificate is not trusted. Set ca_file to the cluster CA (/etc/pve/pve-root-ca.pem)."
	case errors.As(err, &dnsErr):
		return types.ConnectionStatusNotFound, "Host not found", "The API host name did not resolve."
	case errors.As(err, &netErr) && netErr.Timeout():
		return types.ConnectionStatusTimeout, "Timed out", "The Proxmox API did not respond in time."
	default:
		return types.ConnectionStatusUnavailable, "Unavailable", fmt.Sprintf("Could not reach the Proxmox API: %v", err)
	}
}
