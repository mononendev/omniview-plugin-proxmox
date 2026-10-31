import { useState } from 'react';
import { useConnection, usePluginRouter } from '@omniviewdev/runtime';

import { PLUGIN_ID, connectionPath } from '../constants';

/**
 * Connect/disconnect/open actions for one Proxmox connection. `isConnected`
 * comes from the caller's single `useConnectionStatus()` subscription.
 * Starting checks the API token, so a rejected token or an unreachable API
 * fails the start; the runtime reports start and stop failures.
 */
export function useProxmoxConnection(connectionId: string, isConnected: boolean) {
  const { navigate } = usePluginRouter();
  const { startConnection, stopConnection } = useConnection({
    pluginID: PLUGIN_ID,
    connectionID: connectionId,
  });
  const [pending, setPending] = useState(false);

  const connect = (openOnSuccess: boolean) => {
    setPending(true);
    startConnection()
      .then(() => {
        if (openOnSuccess) navigate(connectionPath(connectionId));
      })
      .catch(() => undefined)
      .finally(() => setPending(false));
  };

  const disconnect = () => {
    setPending(true);
    stopConnection()
      .catch(() => undefined)
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
