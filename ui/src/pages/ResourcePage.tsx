import { useResourceTypes } from '@omniviewdev/runtime';
import { Navigate, useParams } from 'react-router-dom';

import ResourceTable from '../components/ResourceTable';
import { DEFAULT_RESOURCE_KEY, PLUGIN_ID } from '../constants';

/** `/connection/:connectionId/resources/:resourceKey` — any backend-registered kind. */
export default function ResourcePage() {
  const { connectionId = '', resourceKey = '' } = useParams<{ connectionId: string; resourceKey: string }>();
  // Same query as the connection sidebar, so this is served from its cache.
  const { types } = useResourceTypes({ pluginID: PLUGIN_ID, connectionID: connectionId });
  const label = types.data?.[resourceKey]?.label || resourceKey.split('::').pop() || resourceKey;

  // Remount per kind so filter, sort and selection never leak between tables.
  return <ResourceTable key={`${connectionId}/${resourceKey}`} connectionId={connectionId} resourceKey={resourceKey} label={label} />;
}

/** Connection index: open the default kind. */
export function ConnectionIndexRedirect() {
  return <Navigate to={`resources/${encodeURIComponent(DEFAULT_RESOURCE_KEY)}`} replace />;
}
