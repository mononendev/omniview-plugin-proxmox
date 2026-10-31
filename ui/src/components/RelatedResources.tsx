import { EmptyState, Spinner, Stack, StatusDot, Table, Text } from '@omniviewdev/base-ui';
import { ResourceIcon, useRelatedResources } from '@omniviewdev/runtime';
import { LuWaypoints } from 'react-icons/lu';

import { PLUGIN_ID, RESOURCE_KEYS } from '../constants';
import { useClusterObjects } from '../hooks/useClusterObjects';
import { type ProxmoxEntry, errorMessage } from '../hooks/useProxmoxResources';
import { columnStatus, displayName, kindFor } from '../utils/columns';

const rootStyle = { width: '100%' } as const;
const centerStyle = { display: 'flex', justifyContent: 'center', padding: '32px 0' } as const;
const sectionTitleStyle = { margin: '4px 0' } as const;
const statusColumn = { field: 'status', header: 'Status', status: true };
const statusCellStyle = { width: 120 } as const;

/**
 * Related kinds in display order. Each pair of Proxmox kinds is linked by one
 * relationship (guest runs on node, guest stores disks on storage, storage is
 * attached to node), so grouping by kind is unambiguous.
 */
const GROUPS = [
  { resourceKey: RESOURCE_KEYS.node, title: 'Node' },
  { resourceKey: RESOURCE_KEYS.storage, title: 'Storage' },
  { resourceKey: RESOURCE_KEYS.virtualMachine, title: 'Virtual machines' },
  { resourceKey: RESOURCE_KEYS.container, title: 'Containers' },
];

type Props = {
  /** The object whose relations are shown. */
  entry: ProxmoxEntry;
  onOpen: (entry: ProxmoxEntry) => void;
};

/**
 * Drawer view listing what the host's relationship graph links to one entry:
 * a guest's node and storage, a node's guests and storage, a storage entry's
 * node and the guests with disks on it.
 */
export default function RelatedResources({ entry, onOpen }: Props) {
  const { route, record } = entry.record;
  const { related, isLoading, error } = useRelatedResources({
    pluginID: PLUGIN_ID,
    reference: record.reference,
    route,
  });
  const { objects } = useClusterObjects(route.connection);

  if (isLoading) {
    return (
      <div style={centerStyle}>
        <Spinner size="sm" />
      </div>
    );
  }
  if (error) {
    return (
      <EmptyState
        size="sm"
        icon={<LuWaypoints />}
        title="Could not load related resources"
        description={errorMessage(error)}
      />
    );
  }

  const groups = GROUPS.map((group) => {
    const kind = kindFor(group.resourceKey);
    const ids = [
      ...new Set(
        related
          .filter((r) => r.resource.reference.address.type === group.resourceKey)
          .map((r) => r.resource.reference.address.id),
      ),
    ];
    const rows = ids.map((id) => {
      const object = objects.get(id);
      return { id, object, data: object?.data, name: object ? displayName(object.data, kind) : id };
    });
    rows.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    return { ...group, rows };
  }).filter((group) => group.rows.length > 0);

  if (groups.length === 0) {
    return (
      <EmptyState
        size="sm"
        icon={<LuWaypoints />}
        title="No related resources"
        description="Guests are linked to their node and to the storage holding their disks, storage to its node."
      />
    );
  }

  return (
    <Stack direction="column" spacing={3} style={rootStyle}>
      {groups.map((group) => (
        <section key={group.resourceKey}>
          <Text weight="semibold" size="sm" style={sectionTitleStyle}>
            {`${group.title} (${group.rows.length})`}
          </Text>
          <Table size="sm" hoverable>
            <Table.Body>
              {group.rows.map((row) => {
                const status = row.data ? columnStatus(row.data.status, statusColumn) : undefined;
                return (
                  <Table.Row
                    key={row.id}
                    interactive={!!row.object}
                    onClick={row.object ? () => onOpen(row.object!) : undefined}
                  >
                    <Table.Cell truncate>
                      <Stack direction="row" align="center" spacing={2}>
                        <ResourceIcon pluginId={PLUGIN_ID} resourceKey={group.resourceKey} size={14} />
                        <Text size="sm">{row.name}</Text>
                      </Stack>
                    </Table.Cell>
                    <Table.Cell style={statusCellStyle}>
                      {status ? <StatusDot status={status} label={String(row.data?.status)} size="sm" /> : null}
                    </Table.Cell>
                  </Table.Row>
                );
              })}
            </Table.Body>
          </Table>
        </section>
      ))}
    </Stack>
  );
}

