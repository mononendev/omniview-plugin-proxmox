import { useState } from 'react';
import { useConnection, usePluginRouter, useSnackbar } from '@omniviewdev/runtime';
import { types } from '@omniviewdev/runtime/models';

import { PLUGIN_ID, connectionPath } from '../constants';

/**
 * Connect/disconnect/open actions for one Proxmox connection. `isConnected`
 * comes from the caller's single `useConnectionStatus()` subscription.
 */
export function useProxmoxConnection(connectionId: string, displayName: string, isConnected: boolean) {
  const { navigate } = usePluginRouter();
  const { showSnackbar } = useSnackbar();
  const { startConnection, stopConnection } = useConnection({
    pluginID: PLUGIN_ID,
    connectionID: connectionId,
  });
  const [pending, setPending] = useState(false);

  const reportError = (message: string) => (err: unknown) => {
    showSnackbar({ status: 'error', message, details: err instanceof Error ? err.message : String(err) });
  };

  const connect = (openOnSuccess: boolean) => {
    setPending(true);
    startConnection()
      .then((status) => {
        if (status.status === types.ConnectionStatusCode.ConnectionStatusConnected) {
          if (openOnSuccess) navigate(connectionPath(connectionId));
          return;
        }
        const unauthorized = status.status === types.ConnectionStatusCode.ConnectionStatusUnauthorized;
        showSnackbar({
          status: unauthorized ? 'warning' : 'error',
          message: unauthorized
            ? `Proxmox rejected the API token for '${displayName}'`
            : `Failed to connect to '${displayName}'`,
          details: status.details || status.error,
        });
      })
      .catch(reportError(`Failed to connect to '${displayName}'`))
      .finally(() => setPending(false));
  };

  const disconnect = () => {
    setPending(true);
    stopConnection()
      .catch(reportError(`Failed to disconnect '${displayName}'`))
      .finally(() => setPending(false));
  };

  return {
    pending,
    connect: () => connect(false),
    /** Opens the connection page, connecting first when needed. */
    open: () => (isConnected ? navigate(connectionPath(connectionId)) : connect(true)),
    disconnect,
  };
}
