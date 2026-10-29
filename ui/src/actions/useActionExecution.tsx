import { useMemo, useState } from 'react';
import {
  useConfirmationModal,
  useExec,
  useExecuteAction,
  useLogs,
  useResourceMutations,
  useSnackbar,
} from '@omniviewdev/runtime';

import { PLUGIN_ID } from '../constants';
import type { ActionDeps, ActionTarget } from './actionDeps';
import DeleteGuestDialog from './DeleteGuestDialog';
import EditConfigDialog from './EditConfigDialog';

type Options = {
  connectionID: string;
  resourceKey: string;
  /** Runs after a delete from this mount point succeeds, e.g. to close the drawer. */
  onDeleted?: () => void;
};

/**
 * The runtime hooks actions run with, for one resource kind, plus the
 * dialogs they open. Render `dialogs` wherever the hook is used.
 */
export function useActionExecution({ connectionID, resourceKey, onDeleted }: Options) {
  const { show } = useConfirmationModal();
  const { executeAction } = useExecuteAction({ pluginID: PLUGIN_ID, connectionID, resourceKey });
  const { createSession } = useExec({ pluginID: PLUGIN_ID });
  const { createLogSession } = useLogs({ pluginID: PLUGIN_ID });
  const { update, remove } = useResourceMutations({ pluginID: PLUGIN_ID });
  const { showSnackbar } = useSnackbar();
  const [editTarget, setEditTarget] = useState<ActionTarget>();
  const [deleteTarget, setDeleteTarget] = useState<ActionTarget>();

  const deps = useMemo<ActionDeps>(
    () => ({
      show,
      executeAction,
      createExecSession: createSession,
      createLogSession,
      notify: (message) => showSnackbar(message, 'info'),
      openEditConfig: setEditTarget,
      openDelete: setDeleteTarget,
    }),
    [show, executeAction, createSession, createLogSession, showSnackbar],
  );

  const dialogs = (
    <>
      {editTarget && (
        <EditConfigDialog target={editTarget} update={update} onClose={() => setEditTarget(undefined)} />
      )}
      {deleteTarget && (
        <DeleteGuestDialog
          target={deleteTarget}
          remove={remove}
          onClose={() => setDeleteTarget(undefined)}
          onDeleted={() => onDeleted?.()}
        />
      )}
    </>
  );

  return { deps, dialogs };
}
