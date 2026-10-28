import { useMemo, useState } from 'react';
import {
  CodeBlock,
  EmptyState,
  IconButton,
  SearchInput,
  SortableHeader,
  Spinner,
  Stack,
  StatusDot,
  Table,
  Text,
  useSortableTable,
  type SortableColumnDef,
} from '@omniviewdev/base-ui';
import { ResourceIcon, useResources, useRightDrawer, type DrawerComponent } from '@omniviewdev/runtime';
import { ResourceClient } from '@omniviewdev/runtime/api';
import { useQuery } from '@tanstack/react-query';
import { LuBraces, LuCircleAlert, LuRefreshCw, LuSquareChartGantt } from 'react-icons/lu';

import { PLUGIN_ID } from '../constants';
import {
  type ColumnDefinition,
  type ProxmoxObject,
  columnAlign,
  columnStatus,
  columnValue,
  displayName,
  formatValue,
  sortValue,
  visibleColumns,
} from '../utils/columns';
import ResourceOverview from './ResourceOverview';

const rootStyle = { height: '100%', minHeight: 0, padding: 8 } as const;
const toolbarStyle = { padding: '0 4px' } as const;
const tableContainerStyle = { flex: 1, minHeight: 0, overflow: 'auto' } as const;
const centerStyle = { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' } as const;

type Props = {
  connectionId: string;
  resourceKey: string;
  label: string;
};

/**
 * Definition-driven table for one Proxmox resource kind. Columns, formatting
 * and status colours come from the backend's ResourceDefinition; rows are the
 * raw `/cluster/resources` entries keyed by their opaque Proxmox `id`.
 */
export default function ResourceTable({ connectionId, resourceKey, label }: Props) {
  const [filter, setFilter] = useState('');

  const { resources } = useResources({
    pluginID: PLUGIN_ID,
    connectionID: connectionId,
    resourceKey,
    idAccessor: 'id',
  });
  const definition = useQuery({
    queryKey: [PLUGIN_ID, 'resource_definition', resourceKey],
    queryFn: () => ResourceClient.GetResourceDefinition(PLUGIN_ID, resourceKey),
    retry: false,
    staleTime: Infinity,
  });

  const columns = useMemo(() => visibleColumns(definition.data?.columnDefs), [definition.data]);
  const rows = useMemo(() => {
    const all = (resources.data?.result ?? []) as ProxmoxObject[];
    const needle = filter.trim().toLowerCase();
    if (!needle) return all;
    return all.filter(
      (row) =>
        row.id.toLowerCase().includes(needle) ||
        columns.some((col) => formatValue(columnValue(row, col), col).toLowerCase().includes(needle)),
    );
  }, [resources.data, filter, columns]);

  const error = resources.error ?? definition.error;
  const loading = resources.isLoading || definition.isLoading;

  let body;
  if (error) {
    body = (
      <div style={centerStyle}>
        <EmptyState icon={<LuCircleAlert />} title={`Failed to load ${label.toLowerCase()}`} description={error.message} />
      </div>
    );
  } else if (loading) {
    body = (
      <div style={centerStyle}>
        <Spinner />
      </div>
    );
  } else if (rows.length === 0 && !filter) {
    body = (
      <div style={centerStyle}>
        <EmptyState
          icon={<ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={24} />}
          title={`No ${label.toLowerCase()}`}
        />
      </div>
    );
  } else {
    // Mounted once the definition has loaded so the default sort uses its first column,
    // and kept mounted while filtering so the chosen sort survives an empty result.
    body = <SortedTable connectionId={connectionId} resourceKey={resourceKey} columns={columns} rows={rows} />;
  }

  return (
    <Stack direction="column" spacing={2} style={rootStyle}>
      <Stack direction="row" align="center" justify="between" spacing={2} style={toolbarStyle}>
        <Stack direction="row" align="center" spacing={2}>
          <ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={18} />
          <Text weight="semibold">{label}</Text>
          {!loading && !error && (
            <Text size="sm" tone="muted">
              {rows.length}
            </Text>
          )}
        </Stack>
        <Stack direction="row" align="center" spacing={1}>
          <SearchInput
            size="sm"
            placeholder={`Filter ${label.toLowerCase()}`}
            value={filter}
            onValueChange={setFilter}
            clearable
          />
          <IconButton
            size="sm"
            variant="ghost"
            color="neutral"
            aria-label="Refresh"
            onClick={() => void resources.refetch()}
          >
            <LuRefreshCw />
          </IconButton>
        </Stack>
      </Stack>
      {body}
    </Stack>
  );
}

type SortedTableProps = {
  connectionId: string;
  resourceKey: string;
  columns: ColumnDefinition[];
  rows: ProxmoxObject[];
};

function SortedTable({ connectionId, resourceKey, columns, rows }: SortedTableProps) {
  const { openDrawer, isOpen } = useRightDrawer();
  const [selectedId, setSelectedId] = useState<string>();

  const sortColumns = useMemo<Array<SortableColumnDef<ProxmoxObject>>>(
    () =>
      columns.map((col) => ({
        id: col.id,
        header: col.header,
        accessor: (row) => sortValue(columnValue(row, col)),
      })),
    [columns],
  );
  const { sortedData, sort, onSort } = useSortableTable({ data: rows, columns: sortColumns });

  const openResource = (row: ProxmoxObject) => {
    setSelectedId(row.id);
    const drawer: DrawerComponent<ProxmoxObject> = {
      title: displayName(row),
      icon: <ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={16} />,
      views: [
        {
          title: 'Overview',
          icon: <LuSquareChartGantt />,
          component: (ctx) => <ResourceOverview data={ctx.data} columns={columns} />,
        },
        {
          title: 'JSON',
          icon: <LuBraces />,
          component: (ctx) => <CodeBlock language="json" code={JSON.stringify(ctx.data, null, 2)} copyable wrap />,
        },
      ],
      actions: [],
    };
    openDrawer(drawer, {
      data: row,
      resource: { id: row.id, name: displayName(row), key: resourceKey, connectionID: connectionId, pluginID: PLUGIN_ID },
    });
  };

  return (
    <Table.Container style={tableContainerStyle}>
      <Table size="sm" stickyHeader hoverable>
        <Table.Head>
          <Table.Row>
            {columns.map((col) => (
              <Table.HeaderCell key={col.id} align={columnAlign(col)} style={col.width ? { width: col.width } : undefined}>
                <SortableHeader columnId={col.id} sort={sort} onSort={onSort}>
                  {col.header}
                </SortableHeader>
              </Table.HeaderCell>
            ))}
          </Table.Row>
        </Table.Head>
        <Table.Body>
          {sortedData.map((row) => (
            <Table.Row key={row.id} interactive selected={isOpen && row.id === selectedId} onClick={() => openResource(row)}>
              {columns.map((col) => (
                <ResourceCell key={col.id} row={row} col={col} />
              ))}
            </Table.Row>
          ))}
        </Table.Body>
      </Table>
    </Table.Container>
  );
}

function ResourceCell({ row, col }: { row: ProxmoxObject; col: ColumnDefinition }) {
  const value = columnValue(row, col);
  const text = formatValue(value, col);
  const status = columnStatus(value, col);
  const align = columnAlign(col);
  return (
    <Table.Cell align={align} numeric={align === 'right'} truncate>
      {status && text ? <StatusDot status={status} label={text} size="sm" /> : text}
    </Table.Cell>
  );
}
