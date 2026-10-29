import { type ReactNode, useEffect } from 'react';
import { type DrawerComponent, type DrawerContext, useResources, useRightDrawer } from '@omniviewdev/runtime';

import { PLUGIN_ID } from '../constants';
import { type NativeObject, displayName, kindFor, objectId } from '../utils/columns';
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
  const { deps, dialogs } = useActionExecution({
    connectionID: ctx.resource?.connectionID ?? '',
    resourceKey: ctx.resource?.key ?? '',
    onDeleted: closeDrawer,
  });
  // The host calls header actions with this same context object.
  bindActionDeps(ctx, deps);

  return (
    <>
      {live && <LiveData ctx={ctx} drawer={drawer} />}
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
  const resourceKey = ctx.resource?.key ?? '';
  const kind = kindFor(resourceKey);
  const { resources } = useResources({
    pluginID: PLUGIN_ID,
    connectionID: ctx.resource?.connectionID ?? '',
    resourceKey,
    idAccessor: kind.idField,
  });

  useEffect(() => {
    if (!isOpen || !ctx.resource || !resources.data) return;
    const id = ctx.resource.id;
    const fresh = ((resources.data.result ?? []) as NativeObject[]).find((row) => objectId(row, kind) === id);
    if (!fresh || JSON.stringify(fresh) === JSON.stringify(ctx.data)) return;
    // A rename shows in the title and in the dialogs' target name too.
    const name = displayName(fresh, kind);
    openDrawer({ ...drawer, title: name }, { ...ctx, data: fresh, resource: { ...ctx.resource, name } });
  }, [isOpen, resources.data, ctx, kind, drawer, openDrawer]);

  return null;
}
