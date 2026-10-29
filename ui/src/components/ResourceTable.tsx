import { useMemo, useState } from 'react';
import {
  ContextMenu,
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
import { type DrawerContext, ResourceIcon, useResources, useRightDrawer } from '@omniviewdev/runtime';
import { LuCircleAlert, LuRefreshCw } from 'react-icons/lu';

import { bindActionDeps } from '../actions/actionDeps';
import { resourceActions } from '../actions/resourceActions';
import { useActionExecution } from '../actions/useActionExecution';
import { PLUGIN_ID } from '../constants';
import { useOpenResource } from '../hooks/useOpenResource';
import { RowActionsButton, RowContextMenuPopup, menuEntries } from './RowActions';
import {
  type Column,
  type Kind,
  type NativeObject,
  cellValue,
  columnStatus,
  displayName,
  formatValue,
  isNumeric,
  kindFor,
  objectId,
  sortValue,
} from '../utils/columns';

const rootStyle = { height: '100%', minHeight: 0, padding: 8 } as const;
const toolbarStyle = { padding: '0 4px' } as const;
const tableContainerStyle = { flex: 1, minHeight: 0, overflow: 'auto' } as const;
const centerStyle = { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' } as const;
const actionsCellStyle = { width: 36, paddingBlock: 0 } as const;
const rowGroupStyle = { display: 'table-row-group' } as const;

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
        kind.columns.some((col) => formatValue(cellValue(row, col), col).toLowerCase().includes(needle)),
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
  const { isOpen } = useRightDrawer();
  const open = useOpenResource(connectionId);
  const [selectedId, setSelectedId] = useState<string>();
  const [menuRowId, setMenuRowId] = useState<string>();
  const { columns } = kind;
  const actions = resourceActions(resourceKey);
  const { deps, dialogs } = useActionExecution({ connectionID: connectionId, resourceKey });

  const sortColumns = useMemo<Array<SortableColumnDef<NativeObject>>>(
    () =>
      columns.map((col) => ({
        id: col.field,
        header: col.header,
        accessor: (row) => sortValue(cellValue(row, col)),
      })),
    [columns],
  );
  const { sortedData, sort, onSort } = useSortableTable({ data: rows, columns: sortColumns });

  // Row menus run the drawer's actions against a context bound to this table's handlers.
  const contexts = useMemo(() => {
    const byId = new Map<string, DrawerContext<NativeObject>>();
    for (const row of rows) {
      const id = objectId(row, kind);
      const ctx = {
        data: row,
        resource: { id, name: displayName(row, kind), key: resourceKey, connectionID: connectionId, pluginID: PLUGIN_ID },
      };
      byId.set(id, bindActionDeps(ctx, deps));
    }
    return byId;
  }, [rows, kind, resourceKey, connectionId, deps]);

  const openResource = (row: NativeObject) => {
    setSelectedId(objectId(row, kind));
    open(resourceKey, row);
  };

  const hasActions = actions.length > 0;
  const tableRows = sortedData.map((row) => {
    const id = objectId(row, kind);
    const ctx = contexts.get(id);
    return (
      <Table.Row
        key={id}
        interactive
        selected={isOpen && id === selectedId}
        onClick={() => openResource(row)}
        onContextMenu={hasActions ? () => setMenuRowId(id) : undefined}
      >
        {columns.map((col) => (
          <ResourceCell key={col.field} row={row} col={col} />
        ))}
        {hasActions && (
          <Table.Cell align="right" style={actionsCellStyle}>
            {ctx && <RowActionsButton entries={menuEntries(actions, ctx)} />}
          </Table.Cell>
        )}
      </Table.Row>
    );
  });
  const menuCtx = menuRowId === undefined ? undefined : contexts.get(menuRowId);

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
            {hasActions && <Table.HeaderCell style={actionsCellStyle} aria-label="Actions" />}
          </Table.Row>
        </Table.Head>
        {hasActions ? (
          <ContextMenu.Root>
            {/* The trigger's own class makes it a block; keep the tbody a row group. */}
            <ContextMenu.Trigger render={<Table.Body />} style={rowGroupStyle}>
              {tableRows}
            </ContextMenu.Trigger>
            <RowContextMenuPopup entries={menuCtx ? menuEntries(actions, menuCtx) : []} />
          </ContextMenu.Root>
        ) : (
          <Table.Body>{tableRows}</Table.Body>
        )}
      </Table>
      {dialogs}
    </Table.Container>
  );
}

function ResourceCell({ row, col }: { row: NativeObject; col: Column }) {
  const value = cellValue(row, col);
  const text = formatValue(value, col);
  const status = columnStatus(value, col);
  return (
    <Table.Cell align={isNumeric(col) ? 'right' : 'left'} numeric={isNumeric(col)} truncate>
      {status && text ? <StatusDot status={status} label={text} size="sm" /> : text}
    </Table.Cell>
  );
}
