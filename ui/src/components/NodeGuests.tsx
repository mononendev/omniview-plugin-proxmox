import { EmptyState, Spinner, Stack, StatusDot, Table, Text } from '@omniviewdev/base-ui';
import { ResourceIcon } from '@omniviewdev/runtime';
import { LuServer } from 'react-icons/lu';

import { PLUGIN_ID, RESOURCE_KEYS } from '../constants';
import { useClusterObjects } from '../hooks/useClusterObjects';
import { type ProxmoxObject, displayName, formatBytes, kindFor, statusOf } from '../utils/columns';

const rootStyle = { width: '100%' } as const;
const centerStyle = { display: 'flex', justifyContent: 'center', padding: '32px 0' } as const;
const sectionTitleStyle = { margin: '4px 0' } as const;
const statusCellStyle = { width: 96 } as const;
const cpuCellStyle = { width: 72 } as const;
const memoryCellStyle = { width: 150 } as const;

type Props = {
  connectionId: string;
  /** The node's name (`node` field). */
  node: string;
  onOpen: (resourceKey: string, row: ProxmoxObject) => void;
};

type Guest = { resourceKey: string; data: ProxmoxObject; name: string; running: boolean };

/** Drawer view of a node: the guests placed on it, running first, with live CPU and memory use. */
export default function NodeGuests({ connectionId, node, onOpen }: Props) {
  const { virtualMachines, containers, isLoading, error } = useClusterObjects(connectionId);

  if (isLoading) {
    return (
      <div style={centerStyle}>
        <Spinner size="sm" />
      </div>
    );
  }
  if (error) {
    return <EmptyState size="sm" icon={<LuServer />} title="Could not load guests" description={error.message} />;
  }

  const guests: Guest[] = [
    ...virtualMachines.map((data) => ({ resourceKey: RESOURCE_KEYS.virtualMachine, data })),
    ...containers.map((data) => ({ resourceKey: RESOURCE_KEYS.container, data })),
  ]
    .filter(({ data }) => data.node === node)
    .map((g) => ({ ...g, name: displayName(g.data, kindFor(g.resourceKey)), running: g.data.status === 'running' }))
    .sort((a, b) => Number(b.running) - Number(a.running) || a.name.localeCompare(b.name, undefined, { numeric: true }));

  if (guests.length === 0) {
    return <EmptyState size="sm" icon={<LuServer />} title="No guests on this node" />;
  }

  const running = guests.filter((g) => g.running).length;
  return (
    <Stack direction="column" spacing={2} style={rootStyle}>
      <Text weight="semibold" size="sm" style={sectionTitleStyle}>
        {`${guests.length} guests, ${running} running`}
      </Text>
      <Table size="sm" hoverable>
        <Table.Head>
          <Table.Row>
            <Table.HeaderCell>Guest</Table.HeaderCell>
            <Table.HeaderCell style={statusCellStyle}>Status</Table.HeaderCell>
            <Table.HeaderCell align="right" style={cpuCellStyle}>
              CPU
            </Table.HeaderCell>
            <Table.HeaderCell align="right" style={memoryCellStyle}>
              Memory
            </Table.HeaderCell>
          </Table.Row>
        </Table.Head>
        <Table.Body>
          {guests.map((g) => {
            const { cpu, maxcpu, mem, maxmem, status } = g.data;
            return (
              <Table.Row key={g.data.id} interactive onClick={() => onOpen(g.resourceKey, g.data)}>
                <Table.Cell truncate>
                  <Stack direction="row" align="center" spacing={2}>
                    <ResourceIcon pluginId={PLUGIN_ID} resourceKey={g.resourceKey} size={14} />
                    <Text size="sm">{g.name}</Text>
                  </Stack>
                </Table.Cell>
                <Table.Cell style={statusCellStyle}>
                  <StatusDot status={statusOf(status)} label={String(status ?? '')} size="sm" />
                </Table.Cell>
                <Table.Cell
                  align="right"
                  numeric
                  style={cpuCellStyle}
                  title={typeof maxcpu === 'number' ? `of ${maxcpu} vCPUs` : undefined}
                >
                  {g.running && typeof cpu === 'number' ? `${(cpu * 100).toFixed(1)} %` : '—'}
                </Table.Cell>
                <Table.Cell align="right" numeric style={memoryCellStyle}>
                  {g.running && typeof mem === 'number' && typeof maxmem === 'number'
                    ? `${formatBytes(mem)} / ${formatBytes(maxmem)}`
                    : '—'}
                </Table.Cell>
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table>
    </Stack>
  );
}
