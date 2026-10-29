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
import { LuBraces, LuCircleAlert, LuRefreshCw, LuSquareChartGantt } from 'react-icons/lu';

import { PLUGIN_ID } from '../constants';
import {
  type Column,
  type Kind,
  type NativeObject,
  columnStatus,
  displayName,
  fieldValue,
  formatValue,
  isNumeric,
  kindFor,
  objectId,
  sortValue,
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
 * Table for one Proxmox resource kind. Rows are the raw native objects keyed
 * by the kind's ID field; columns come from kindFor().
 */
export default function ResourceTable({ connectionId, resourceKey, label }: Props) {
  const [filter, setFilter] = useState('');
  const kind = kindFor(resourceKey);

  const { resources } = useResources({
    pluginID: PLUGIN_ID,
    connectionID: connectionId,
    resourceKey,
    idAccessor: kind.idField,
  });
  const rows = useMemo(() => {
    const all = (resources.data?.result ?? []) as NativeObject[];
    const needle = filter.trim().toLowerCase();
    if (!needle) return all;
    return all.filter(
      (row) =>
        objectId(row, kind).toLowerCase().includes(needle) ||
        kind.columns.some((col) => formatValue(fieldValue(row, col.field), col).toLowerCase().includes(needle)),
    );
  }, [resources.data, filter, kind]);

  let body;
  if (resources.error) {
    body = (
      <div style={centerStyle}>
        <EmptyState
          icon={<LuCircleAlert />}
          title={`Failed to load ${label.toLowerCase()}`}
          description={resources.error.message}
        />
      </div>
    );
  } else if (resources.isLoading) {
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
    // Kept mounted while filtering so the chosen sort survives an empty result.
    body = <SortedTable connectionId={connectionId} resourceKey={resourceKey} kind={kind} rows={rows} />;
  }

  return (
    <Stack direction="column" spacing={2} style={rootStyle}>
      <Stack direction="row" align="center" justify="between" spacing={2} style={toolbarStyle}>
        <Stack direction="row" align="center" spacing={2}>
          <ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={18} />
          <Text weight="semibold">{label}</Text>
          {!resources.isLoading && !resources.error && (
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
  kind: Kind;
  rows: NativeObject[];
};

function SortedTable({ connectionId, resourceKey, kind, rows }: SortedTableProps) {
  const { openDrawer, isOpen } = useRightDrawer();
  const [selectedId, setSelectedId] = useState<string>();
  const { columns } = kind;

  const sortColumns = useMemo<Array<SortableColumnDef<NativeObject>>>(
    () =>
      columns.map((col) => ({
        id: col.field,
        header: col.header,
        accessor: (row) => sortValue(fieldValue(row, col.field)),
      })),
    [columns],
  );
  const { sortedData, sort, onSort } = useSortableTable({ data: rows, columns: sortColumns });

  const openResource = (row: NativeObject) => {
    const id = objectId(row, kind);
    const name = displayName(row, kind);
    setSelectedId(id);
    const drawer: DrawerComponent<NativeObject> = {
      title: name,
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
      resource: { id, name, key: resourceKey, connectionID: connectionId, pluginID: PLUGIN_ID },
    });
  };

  return (
    <Table.Container style={tableContainerStyle}>
      <Table size="sm" stickyHeader hoverable>
        <Table.Head>
          <Table.Row>
            {columns.map((col) => (
              <Table.HeaderCell key={col.field} align={isNumeric(col) ? 'right' : 'left'}>
                <SortableHeader columnId={col.field} sort={sort} onSort={onSort}>
                  {col.header}
                </SortableHeader>
              </Table.HeaderCell>
            ))}
          </Table.Row>
        </Table.Head>
        <Table.Body>
          {sortedData.map((row) => {
            const id = objectId(row, kind);
            return (
              <Table.Row key={id} interactive selected={isOpen && id === selectedId} onClick={() => openResource(row)}>
                {columns.map((col) => (
                  <ResourceCell key={col.field} row={row} col={col} />
                ))}
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table>
    </Table.Container>
  );
}

function ResourceCell({ row, col }: { row: NativeObject; col: Column }) {
  const value = fieldValue(row, col.field);
  const text = formatValue(value, col);
  const status = columnStatus(value, col);
  return (
    <Table.Cell align={isNumeric(col) ? 'right' : 'left'} numeric={isNumeric(col)} truncate>
      {status && text ? <StatusDot status={status} label={text} size="sm" /> : text}
    </Table.Cell>
  );
}
