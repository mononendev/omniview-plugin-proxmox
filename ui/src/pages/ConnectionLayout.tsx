import { useEffect, useMemo, useRef } from 'react';
import { IconButton, NavList, Spinner, Stack, StatusDot, Text } from '@omniviewdev/base-ui';
import {
  ResourceIcon,
  useConnection,
  useConnectionStatus,
  usePluginRouter,
  useResourceTypes,
} from '@omniviewdev/runtime';
import { LuChevronLeft, LuLayoutDashboard, LuNetwork, LuPlug, LuUnplug } from 'react-icons/lu';
import { Outlet, useParams } from 'react-router-dom';

import { CATEGORY_ORDER, PLUGIN_ID, overviewPath, resourcePath, topologyPath } from '../constants';
import { useProxmoxConnection } from '../hooks/useProxmoxConnection';

/** NavList keys of the connection's own pages, also their last path segment; resource keys always contain `::`. */
const OVERVIEW_KEY = 'overview';
const TOPOLOGY_KEY = 'topology';

const rootStyle = { display: 'flex', flexDirection: 'row', height: '100%', minHeight: 0, overflow: 'hidden' } as const;
const sideNavStyle = {
  width: 232,
  flexShrink: 0,
  display: 'flex',
  flexDirection: 'column',
  gap: 8,
  padding: 8,
  borderRight: '1px solid var(--ov-color-border-default)',
  overflow: 'auto',
} as const;
const headerStyle = {
  display: 'flex',
  alignItems: 'center',
  gap: 4,
  padding: 6,
  borderRadius: 4,
  border: '1px solid var(--ov-color-border-default)',
} as const;
const nameStyle = { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' } as const;
const mainStyle = { flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' } as const;

type NavItem = { key: string; label: string };
type NavSection = { category: string; items: NavItem[] };

function categoryRank(category: string): number {
  const rank = CATEGORY_ORDER.indexOf(category);
  return rank === -1 ? CATEGORY_ORDER.length : rank;
}

/** Connection page: resource kinds from the backend grouped by category, the selected kind's table beside it. */
export default function ConnectionLayout() {
  const { connectionId = '' } = useParams<{ connectionId: string }>();
  const { navigate, pluginPath } = usePluginRouter();
  const { connection } = useConnection({ pluginID: PLUGIN_ID, connectionID: connectionId });
  const { types } = useResourceTypes({ pluginID: PLUGIN_ID, connectionID: connectionId });
  const { entries } = useConnectionStatus();

  const name = connection.data?.name || connectionId;
  const isConnected = entries.some((e) => e.pluginID === PLUGIN_ID && e.connectionID === connectionId);
  const { pending, connect, disconnect } = useProxmoxConnection(connectionId, name, isConnected);

  // Connecting runs the backend's Ceph discovery, which adds the Ceph kinds
  // to clusters that run Ceph: reload the kinds once connected.
  const refetchTypes = types.refetch;
  const wasConnected = useRef(isConnected);
  useEffect(() => {
    if (isConnected && !wasConnected.current) void refetchTypes();
    wasConnected.current = isConnected;
  }, [isConnected, refetchTypes]);

  const sections = useMemo<NavSection[]>(() => {
    const byCategory = new Map<string, NavItem[]>();
    for (const meta of Object.values(types.data ?? {})) {
      if (!meta) continue;
      const category = meta.category || 'Other';
      const items = byCategory.get(category) ?? [];
      items.push({ key: `${meta.group}::${meta.version}::${meta.kind}`, label: meta.label || meta.kind });
      byCategory.set(category, items);
    }
    return [...byCategory.entries()]
      .map(([category, items]) => ({ category, items: items.sort((a, b) => a.label.localeCompare(b.label)) }))
      .sort((a, b) => categoryRank(a.category) - categoryRank(b.category) || a.category.localeCompare(b.category));
  }, [types.data]);

  const selectedMatch = /\/resources\/([^/]+)$/.exec(pluginPath);
  const pageMatch = /\/(overview|topology)$/.exec(pluginPath);
  const selectedKey = selectedMatch ? decodeURIComponent(selectedMatch[1]) : pageMatch?.[1];
  const selectedKeys = useMemo(() => new Set(selectedKey ? [selectedKey] : []), [selectedKey]);

  return (
    <div style={rootStyle}>
      <nav style={sideNavStyle} aria-label="Proxmox resources">
        <div style={headerStyle}>
          <IconButton size="sm" variant="ghost" color="neutral" aria-label="All connections" onClick={() => navigate('/')}>
            <LuChevronLeft />
          </IconButton>
          <StatusDot status={pending ? 'pending' : isConnected ? 'success' : 'neutral'} size="sm" />
          <Text size="sm" weight="semibold" style={nameStyle} title={name}>
            {name}
          </Text>
          <IconButton
            size="sm"
            variant="ghost"
            color="neutral"
            disabled={pending}
            aria-label={isConnected ? 'Disconnect' : 'Connect'}
            onClick={isConnected ? disconnect : connect}
          >
            {isConnected ? <LuUnplug /> : <LuPlug />}
          </IconButton>
        </div>
        {types.isLoading ? (
          <Stack direction="row" justify="center">
            <Spinner size="sm" />
          </Stack>
        ) : types.error ? (
          <Text size="sm" tone="danger">
            {types.error.message}
          </Text>
        ) : (
          <NavList
            size="sm"
            selectionMode="single"
            selectedKeys={selectedKeys}
            onSelectedKeysChange={(keys) => {
              const [key] = keys;
              if (key === OVERVIEW_KEY) navigate(overviewPath(connectionId));
              else if (key === TOPOLOGY_KEY) navigate(topologyPath(connectionId));
              else if (key !== undefined) navigate(resourcePath(connectionId, String(key)));
            }}
          >
            <NavList.Viewport>
              <NavList.Group>
                <NavList.GroupHeader>Overview</NavList.GroupHeader>
                <NavList.GroupItems>
                  <NavList.Item itemKey={OVERVIEW_KEY} textValue="Summary">
                    <NavList.ItemIcon>
                      <LuLayoutDashboard size={16} />
                    </NavList.ItemIcon>
                    <NavList.ItemLabel>Summary</NavList.ItemLabel>
                  </NavList.Item>
                  <NavList.Item itemKey={TOPOLOGY_KEY} textValue="Topology">
                    <NavList.ItemIcon>
                      <LuNetwork size={16} />
                    </NavList.ItemIcon>
                    <NavList.ItemLabel>Topology</NavList.ItemLabel>
                  </NavList.Item>
                </NavList.GroupItems>
              </NavList.Group>
              {sections.map((section) => (
                <NavList.Group key={section.category}>
                  <NavList.GroupHeader>{section.category}</NavList.GroupHeader>
                  <NavList.GroupItems>
                    {section.items.map((item) => (
                      <NavList.Item key={item.key} itemKey={item.key} textValue={item.label}>
                        <NavList.ItemIcon>
                          <ResourceIcon pluginId={PLUGIN_ID} resourceKey={item.key} size={16} />
                        </NavList.ItemIcon>
                        <NavList.ItemLabel>{item.label}</NavList.ItemLabel>
                      </NavList.Item>
                    ))}
                  </NavList.GroupItems>
                </NavList.Group>
              ))}
            </NavList.Viewport>
          </NavList>
        )}
      </nav>
      <main style={mainStyle}>
        <Outlet />
      </main>
    </div>
  );
}
