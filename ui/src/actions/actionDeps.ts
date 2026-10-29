import type { ConfirmationModalContextType, DrawerContext } from '@omniviewdev/runtime';
import type { ActionResult, SessionOptions } from '@omniviewdev/runtime/models';
import type { ReactNode } from 'react';

import type { NativeObject } from '../utils/columns';

/** The object an action dialog works on. */
export type ActionTarget = {
  connectionID: string;
  resourceKey: string;
  /** Backend ID: `qemu/100`, `node/apollo`, `osd.19`. */
  id: string;
  /** Human label, e.g. `web01 (100)`. */
  name: string;
  data: NativeObject;
};

/** Runs a backend action on one object of the bound resource kind (runtime `useExecuteAction`). */
export type ExecuteAction = (args: { actionID: string; id: string; params?: Record<string, unknown> }) => Promise<
  ActionResult | null
>;

/** Opens a terminal tab (runtime `useExec().createSession`); failures are reported by the runtime. */
export type CreateExecSession = (args: {
  connectionID: string;
  label?: string;
  icon?: string | ReactNode;
  opts: Partial<SessionOptions>;
}) => Promise<void>;

/** Opens a log viewer tab (runtime `useLogs().createLogSession`). */
export type CreateLogSession = (args: {
  connectionID: string;
  resourceKey: string;
  resourceID: string;
  resourceData: Record<string, unknown>;
  label?: string;
  icon?: string | ReactNode;
}) => Promise<unknown>;

/**
 * Hook-backed handlers that actions run with. Drawer actions and row menus
 * are plain objects outside React, so each mount point (drawer view, table)
 * binds its handlers to the DrawerContext it hands to them.
 */
export type ActionDeps = {
  show: ConfirmationModalContextType['show'];
  executeAction: ExecuteAction;
  createExecSession: CreateExecSession;
  createLogSession: CreateLogSession;
  /** Shows a short progress notice while a slow task runs. */
  notify: (message: string) => void;
  openEditConfig: (target: ActionTarget) => void;
  openDelete: (target: ActionTarget) => void;
};

const bound = new WeakMap<DrawerContext, ActionDeps>();

export function bindActionDeps(ctx: DrawerContext, deps: ActionDeps): DrawerContext {
  bound.set(ctx, deps);
  return ctx;
}

/** The handlers bound to `ctx`. Call it inside an action handler. */
export function actionDeps(ctx: DrawerContext): ActionDeps {
  const deps = bound.get(ctx);
  if (!deps) throw new Error(`No action handlers are mounted for ${ctx.resource?.key ?? 'this resource'}`);
  return deps;
}
