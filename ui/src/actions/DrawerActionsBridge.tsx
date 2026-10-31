import { type ReactNode, useEffect } from 'react';
import { type DrawerComponent, type DrawerContext, useRightDrawer } from '@omniviewdev/runtime';

import { drawerContextOf, useProxmoxResources } from '../hooks/useProxmoxResources';
import { type NativeObject, displayName, kindFor } from '../utils/columns';
import { bindActionDeps } from './actionDeps';
import { useActionExecution } from './useActionExecution';

type Props = {
  ctx: DrawerContext<NativeObject>;
  /** The open drawer, re-opened with fresh data when the object changes. */
  drawer: DrawerComponent<NativeObject>;
  /** Keep `ctx.data` current, so status-dependent actions and views follow the object. */
  live: boolean;
  children: ReactNode;
};

/**
 * Wraps every drawer view: binds the header actions' handlers to the
 * drawer's context and renders their dialogs, whichever view is showing.
 */
export default function DrawerActionsBridge({ ctx, drawer, live, children }: Props) {
  const { closeDrawer } = useRightDrawer();
  const { deps, dialogs } = useActionExecution({ onDeleted: closeDrawer });
  // The host calls header actions with this same context object.
  bindActionDeps(ctx, deps);

  return (
    <>
      {live && ctx.resource && <LiveData ctx={ctx} drawer={drawer} />}
      {children}
      {dialogs}
    </>
  );
}

/**
 * The host drawer keeps the object it was opened with. Re-opening it with the
 * list's current copy refreshes header actions and views in place; the
 * drawer keeps its selected view.
 */
function LiveData({ ctx, drawer }: { ctx: DrawerContext<NativeObject>; drawer: DrawerComponent<NativeObject> }) {
  const { openDrawer, isOpen } = useRightDrawer();
  const { reference, route } = ctx.resource!;
  const { type, id } = reference.address;
  const { entries } = useProxmoxResources(route.connection, type);

  useEffect(() => {
    if (!isOpen) return;
    const fresh = entries.find((entry) => entry.id === id);
    // Tagged bodies compare exactly and, unlike decoded ones, never hold a bigint.
    if (!fresh || JSON.stringify(fresh.record.record.body) === JSON.stringify(ctx.resource?.record?.record.body)) return;
    // A rename shows in the title and in the dialogs' target name too.
    const name = displayName(fresh.data, kindFor(type));
    openDrawer({ ...drawer, title: name }, drawerContextOf(fresh, name));
  }, [isOpen, entries, id, type, ctx.resource, drawer, openDrawer]);

  return null;
}
