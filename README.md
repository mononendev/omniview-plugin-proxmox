# Omniview Proxmox VE plugin

Browse Proxmox VE clusters in Omniview: nodes, storage, QEMU virtual machines and LXC containers. Read-only for now.

## Connections

Each entry under `clusters:` in a config file becomes one connection. The default file is
`~/.config/omniview-proxmox/clusters.yaml`; add more under the plugin's **Cluster config files** setting.

```yaml
clusters:
  - id: homelab                     # stable connection ID, unique across all files
    name: Homelab                   # optional display name
    url: https://pve1.lan:8006      # server root, no /api2/json
    token_id: omniview@pve!omniview
    token_secret: 00000000-0000-0000-0000-000000000000
    ca_file: ~/.config/omniview-proxmox/homelab-ca.pem   # copy of /etc/pve/pve-root-ca.pem
    # insecure_skip_verify: true    # last resort for self-signed certificates
```

Keep the file private (`chmod 600`). The token secret is read from the file when a connection
starts; it is never placed in connection data sent to the UI.

Create a read-only token on any cluster node:

```sh
pveum user add omniview@pve
pveum acl modify / --users omniview@pve --roles PVEAuditor
pveum user token add omniview@pve omniview --privsep 0
```

## Resources

| Resource key | Source |
| --- | --- |
| `proxmox::v1::Node` | `GET /cluster/resources?type=node` |
| `proxmox::v1::Storage` | `GET /cluster/resources?type=storage` |
| `proxmox::v1::VirtualMachine` | `GET /cluster/resources?type=vm`, `type == qemu` |
| `proxmox::v1::Container` | `GET /cluster/resources?type=vm`, `type == lxc` |

Objects are the unmodified API entries. Their ID is Proxmox's cluster-unique `id`
(`node/pve1`, `qemu/100`, `storage/pve1/local`); nodes are not mapped onto namespaces.

## Development

```sh
make init       # pnpm install
make test       # go test + UI typecheck
make build      # build/bin/plugin and ui/dist
make package    # dist/proxmox-<os>_<arch>.tar.gz
```

For live development, add this repository to an Omniview development workspace and link it with
`omniview-dev plugin dev link`; see `omniview/docs/development-cli.md`.
