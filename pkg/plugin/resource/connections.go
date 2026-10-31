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

	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"
	"github.com/omniviewdev/plugin-sdk/pkg/types"

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
	dataConfigFile = proxmox.ConnectionDataConfigFile
)

// lifecycle owns one API client per started connection. Starting a
// connection checks the API token and records whether the cluster runs Ceph.
func lifecycle(configFiles func() ([]string, error), ceph *cephState) *resourcev2.Lifecycle[proxmox.Client] {
	return &resourcev2.Lifecycle[proxmox.Client]{
		LoadConnections: func(context.Context) ([]types.Connection, error) {
			files, err := configFiles()
			if err != nil {
				return nil, fmt.Errorf("read %s setting: %w", SettingConfigFiles, err)
			}
			return loadConnections(files)
		},
		CreateClient: func(_ context.Context, conn types.Connection) (*proxmox.Client, error) {
			return newClient(&conn)
		},
		DestroyClient: func(_ context.Context, client *proxmox.Client) error {
			client.Close()
			return nil
		},
		CheckConnection: checkConnection,
		OnStart: func(ctx context.Context, conn types.Connection, client *proxmox.Client) error {
			if _, err := client.Version(ctx); err != nil {
				return connectError(err)
			}
			hasCeph, err := probeCeph(ctx, client)
			if err != nil {
				return connectError(err)
			}
			ceph.set(conn.ID, hasCeph)
			return nil
		},
		OnStop: func(_ context.Context, conn types.Connection, _ *proxmox.Client) error {
			ceph.forget(conn.ID)
			return nil
		},
	}
}

// probeCeph reports whether an online node answers `ceph status`. A cluster
// without Ceph is not an error: it has no Ceph kinds.
func probeCeph(ctx context.Context, client *proxmox.Client) (bool, error) {
	_, err := client.CephStatus(ctx)
	var unavailable *proxmox.CephUnavailableError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &unavailable):
		return false, nil
	default:
		return false, err
	}
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

func checkConnection(ctx context.Context, conn types.Connection, client *proxmox.Client) (types.ConnectionStatus, error) {
	status := types.ConnectionStatus{Connection: &conn}
	if _, err := client.Version(ctx); err != nil {
		failure := classifyConnectionError(err)
		status.Status, status.Error, status.Details = failure.status, failure.title, failure.details
		return status, nil
	}
	status.Status = types.ConnectionStatusConnected
	status.Details = "Connected to the Proxmox VE API"
	return status, nil
}

// connectError is a failed connection start, classified like a check.
func connectError(err error) error {
	failure := classifyConnectionError(err)
	return &resourcev2.ResourceOperationError{Err: err, Code: failure.code, Title: failure.title, Message: failure.details}
}

// connectionFailure is a failed API call as a connection status (checks)
// and a resource error code (starts).
type connectionFailure struct {
	status         types.ConnectionStatusCode
	code           string
	title, details string
}

func classifyConnectionError(err error) connectionFailure {
	var (
		dnsErr      *net.DNSError
		netErr      net.Error
		unknownCA   x509.UnknownAuthorityError
		hostname    x509.HostnameError
		certInvalid x509.CertificateInvalidError
	)
	switch {
	case proxmox.IsStatus(err, http.StatusUnauthorized):
		return connectionFailure{types.ConnectionStatusUnauthorized, "UNAUTHORIZED", "Unauthorized",
			"The API token was rejected. Check token_id and token_secret."}
	case proxmox.IsStatus(err, http.StatusForbidden):
		return connectionFailure{types.ConnectionStatusForbidden, "FORBIDDEN", "Forbidden",
			"The API token lacks permission. Grant it at least PVEAuditor on /."}
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &certInvalid):
		return connectionFailure{types.ConnectionStatusError, "CERTIFICATE_ERROR", "Certificate rejected",
			"The server certificate is not trusted. Set ca_file to the cluster CA (/etc/pve/pve-root-ca.pem)."}
	case errors.As(err, &dnsErr):
		return connectionFailure{types.ConnectionStatusNotFound, "CONNECTION_ERROR", "Host not found",
			"The API host name did not resolve."}
	case errors.As(err, &netErr) && netErr.Timeout():
		return connectionFailure{types.ConnectionStatusTimeout, "TIMEOUT", "Timed out",
			"The Proxmox API did not respond in time."}
	default:
		return connectionFailure{types.ConnectionStatusUnavailable, "CONNECTION_ERROR", "Unavailable",
			fmt.Sprintf("Could not reach the Proxmox API: %v", err)}
	}
}
