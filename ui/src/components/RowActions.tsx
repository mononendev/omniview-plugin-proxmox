import { Fragment, type ReactNode, type SyntheticEvent } from 'react';
import { ContextMenu, IconButton, Menu } from '@omniviewdev/base-ui';
import type { DrawerComponentAction, DrawerContext } from '@omniviewdev/runtime';
import { LuEllipsis } from 'react-icons/lu';

import type { NativeObject } from '../utils/columns';

/** One row menu entry, resolved against a row's context. */
export type MenuEntry = {
  key: string;
  label: string;
  icon?: ReactNode;
  danger: boolean;
  disabled: boolean;
  /** Draws a separator above the entry. */
  separated: boolean;
  onClick: () => void;
};

const triggerButtonStyle = { flex: 'none', minHeight: 24, minWidth: 24 } as const;
const stopPropagation = (e: SyntheticEvent) => e.stopPropagation();

/** A kind's actions as menu entries for one row; Delete goes last, set apart, in danger colour. */
export function menuEntries(actions: Array<DrawerComponentAction<NativeObject>>, ctx: DrawerContext<NativeObject>): MenuEntry[] {
  const entries = actions.flatMap((action): MenuEntry[] => {
    const run = action.action;
    if (typeof action.title !== 'string' || !run) return [];
    const enabled = typeof action.enabled === 'function' ? action.enabled(ctx) : action.enabled !== false;
    return [
      {
        key: action.title,
        label: action.title,
        icon: action.icon,
        danger: action.title === 'Delete',
        disabled: !enabled,
        separated: false,
        onClick: () => run(ctx),
      },
    ];
  });
  const rest = entries.filter((e) => !e.danger);
  const danger = entries.filter((e) => e.danger).map((e) => ({ ...e, separated: rest.length > 0 }));
  return [...rest, ...danger];
}

type MenuParts = {
  Item: typeof Menu.Item | typeof ContextMenu.Item;
  Separator: typeof Menu.Separator | typeof ContextMenu.Separator;
};

function renderEntries(entries: MenuEntry[], { Item, Separator }: MenuParts) {
  return entries.map((entry) => (
    <Fragment key={entry.key}>
      {entry.separated && <Separator />}
      <Item
        startDecorator={entry.icon}
        color={entry.danger ? 'danger' : undefined}
        disabled={entry.disabled}
        onClick={entry.onClick}
      >
        {entry.label}
      </Item>
    </Fragment>
  ));
}

/** Trailing "…" button of a table row. */
export function RowActionsButton({ entries }: { entries: MenuEntry[] }) {
  return (
    <Menu.Root>
      <Menu.Trigger
        render={
          <IconButton
            size="sm"
            variant="ghost"
            color="neutral"
            aria-label="Row actions"
            style={triggerButtonStyle}
            onClick={stopPropagation}
          >
            <LuEllipsis />
          </IconButton>
        }
      />
      <Menu.Portal>
        <Menu.Positioner side="bottom" align="end" sideOffset={4}>
          <Menu.Popup onClick={stopPropagation}>
            {renderEntries(entries, { Item: Menu.Item, Separator: Menu.Separator })}
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** Popup of the right-click menu; the trigger is the element `ContextMenu.Trigger` renders. */
export function RowContextMenuPopup({ entries }: { entries: MenuEntry[] }) {
  if (entries.length === 0) return null;
  return (
    <ContextMenu.Portal>
      <ContextMenu.Positioner>
        <ContextMenu.Popup>
          {renderEntries(entries, { Item: ContextMenu.Item, Separator: ContextMenu.Separator })}
        </ContextMenu.Popup>
      </ContextMenu.Positioner>
    </ContextMenu.Portal>
  );
}
