export const PLUGIN_ID = 'proxmox';

export const RESOURCE_KEYS = {
  node: 'proxmox::v1::Node',
  storage: 'proxmox::v1::Storage',
  virtualMachine: 'proxmox::v1::VirtualMachine',
  container: 'proxmox::v1::Container',
  cephCluster: 'proxmox::v1::CephCluster',
  cephPool: 'proxmox::v1::CephPool',
  cephOSD: 'proxmox::v1::CephOSD',
  cephMonitor: 'proxmox::v1::CephMonitor',
  cephManager: 'proxmox::v1::CephManager',
  cephMetadataServer: 'proxmox::v1::CephMetadataServer',
  cephFilesystem: 'proxmox::v1::CephFilesystem',
} as const;

/** Kinds the backend declares relationship-graph edges for. */
export const GRAPH_RESOURCE_KEYS: ReadonlySet<string> = new Set([
  RESOURCE_KEYS.node,
  RESOURCE_KEYS.storage,
  RESOURCE_KEYS.virtualMachine,
  RESOURCE_KEYS.container,
]);

/** Kinds the backend serves RRD metrics for. */
export const METRIC_RESOURCE_KEYS: ReadonlySet<string> = new Set([
  RESOURCE_KEYS.node,
  RESOURCE_KEYS.virtualMachine,
  RESOURCE_KEYS.container,
]);

/** Sidebar order for backend-supplied categories; unknown categories follow alphabetically. */
export const CATEGORY_ORDER = ['Compute', 'Cluster', 'Ceph'];

/** Usage meter colours by percent: green to 60, amber to 85, red above. */
export const USAGE_ZONES = { low: 60, high: 85, optimum: 0 } as const;

/** Ceph capacity meter colours: green to 75, amber to Ceph's default nearfull ratio (85), red above. */
export const CEPH_USAGE_ZONES = { low: 75, high: 85, optimum: 0 } as const;

/** Plugin-relative path of a connection's page. */
export function connectionPath(connectionId: string): string {
  return `/connection/${encodeURIComponent(connectionId)}`;
}

/** Plugin-relative path of a resource kind's table within a connection. */
export function resourcePath(connectionId: string, resourceKey: string): string {
  return `${connectionPath(connectionId)}/resources/${encodeURIComponent(resourceKey)}`;
}

/** Plugin-relative path of a connection's overview, its landing page. */
export function overviewPath(connectionId: string): string {
  return `${connectionPath(connectionId)}/overview`;
}

/** Plugin-relative path of a connection's topology page. */
export function topologyPath(connectionId: string): string {
  return `${connectionPath(connectionId)}/topology`;
}
