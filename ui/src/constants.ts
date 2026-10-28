export const PLUGIN_ID = 'proxmox';

export const RESOURCE_KEYS = {
  node: 'proxmox::v1::Node',
  storage: 'proxmox::v1::Storage',
  virtualMachine: 'proxmox::v1::VirtualMachine',
  container: 'proxmox::v1::Container',
} as const;

/** Resource opened when a connection page has no resource selected. */
export const DEFAULT_RESOURCE_KEY = RESOURCE_KEYS.virtualMachine;

/** Sidebar order for backend-supplied categories; unknown categories follow alphabetically. */
export const CATEGORY_ORDER = ['Compute', 'Cluster'];

/** Plugin-relative path of a connection's page. */
export function connectionPath(connectionId: string): string {
  return `/connection/${encodeURIComponent(connectionId)}`;
}

/** Plugin-relative path of a resource kind's table within a connection. */
export function resourcePath(connectionId: string, resourceKey: string): string {
  return `${connectionPath(connectionId)}/resources/${encodeURIComponent(resourceKey)}`;
}
