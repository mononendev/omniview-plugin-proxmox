import { Button, EmptyState, Spinner, Stack, StatusDot, Table, Text } from '@omniviewdev/base-ui';
import { useConnectionStatus, useConnections } from '@omniviewdev/runtime';
import type { types } from '@omniviewdev/runtime/models';
import { LuCircleAlert, LuPlug, LuUnplug } from 'react-icons/lu';
import { SiProxmox } from 'react-icons/si';

import { PLUGIN_ID } from '../constants';
import { useProxmoxConnection } from '../hooks/useProxmoxConnection';

const pageStyle = { height: '100%', minHeight: 0, padding: 16, overflow: 'auto' } as const;
const centerStyle = { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' } as const;

/** Connection `data` as supplied by the Proxmox backend. */
type ProxmoxConnectionData = {
  url?: string;
  token_id?: string;
  insecure_skip_verify?: boolean;
};

/** Root page: every configured Proxmox VE cluster or standalone node. */
export default function ConnectionsPage() {
  const { connections } = useConnections({ plugin: PLUGIN_ID });
  const { entries } = useConnectionStatus();
  const started = new Set(entries.filter((e) => e.pluginID === PLUGIN_ID).map((e) => e.connectionID));
  const list = [...(connections.data ?? [])].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));

  let body;
  if (connections.isLoading) {
    body = (
      <div style={centerStyle}>
        <Spinner />
      </div>
    );
  } else if (connections.error) {
    body = (
      <div style={centerStyle}>
        <EmptyState icon={<LuCircleAlert />} title="Failed to load connections" description={connections.error.message} />
      </div>
    );
  } else if (list.length === 0) {
    body = (
      <div style={centerStyle}>
        <EmptyState
          icon={<SiProxmox />}
          title="No Proxmox VE clusters configured"
          description="Clusters are read from the config files listed in this plugin's settings."
        />
      </div>
    );
  } else {
    body = (
      <Table.Container>
        <Table size="sm" hoverable>
          <Table.Head>
            <Table.Row>
              <Table.HeaderCell>Name</Table.HeaderCell>
              <Table.HeaderCell>API URL</Table.HeaderCell>
              <Table.HeaderCell>Token</Table.HeaderCell>
              <Table.HeaderCell>Status</Table.HeaderCell>
              <Table.HeaderCell align="right">Actions</Table.HeaderCell>
            </Table.Row>
          </Table.Head>
          <Table.Body>
            {list.map((connection) => (
              <ConnectionRow key={connection.id} connection={connection} isConnected={started.has(connection.id)} />
            ))}
          </Table.Body>
        </Table>
      </Table.Container>
    );
  }

  return (
    <Stack direction="column" spacing={3} style={pageStyle}>
      <Stack direction="row" align="center" spacing={2}>
        <SiProxmox size={22} />
        <Text size="lg" weight="semibold">
          Proxmox VE
        </Text>
      </Stack>
      {body}
    </Stack>
  );
}

function ConnectionRow({ connection, isConnected }: { connection: types.Connection; isConnected: boolean }) {
  const name = connection.name || connection.id;
  const data = connection.data as ProxmoxConnectionData;
  const { pending, open, connect, disconnect } = useProxmoxConnection(connection.id, name, isConnected);

  return (
    <Table.Row interactive onClick={pending ? undefined : open}>
      <Table.Cell>
        <Text weight="medium">{name}</Text>
        {connection.description && (
          <Text size="xs" tone="muted">
            {connection.description}
          </Text>
        )}
      </Table.Cell>
      <Table.Cell mono truncate>
        {data.url ?? ''}
        {data.insecure_skip_verify && (
          <Text as="span" size="xs" tone="warning">
            {' '}
            (TLS verification disabled)
          </Text>
        )}
      </Table.Cell>
      <Table.Cell mono truncate>
        {data.token_id ?? ''}
      </Table.Cell>
      <Table.Cell>
        <StatusDot
          status={pending ? 'pending' : isConnected ? 'success' : 'neutral'}
          label={pending ? 'Working…' : isConnected ? 'Connected' : 'Disconnected'}
          size="sm"
        />
      </Table.Cell>
      <Table.Cell align="right" onClick={(e) => e.stopPropagation()}>
        {isConnected ? (
          <Button size="sm" variant="ghost" color="neutral" disabled={pending} startDecorator={<LuUnplug />} onClick={disconnect}>
            Disconnect
          </Button>
        ) : (
          <Button size="sm" variant="soft" color="brand" disabled={pending} startDecorator={<LuPlug />} onClick={connect}>
            Connect
          </Button>
        )}
      </Table.Cell>
    </Table.Row>
  );
}
