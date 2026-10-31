import { useResourceTypes } from '@omniviewdev/runtime';
import { useParams } from 'react-router';

import ResourceTable from '../components/ResourceTable';
import { PLUGIN_ID } from '../constants';

/** `/connection/:connectionId/resources/:resourceKey` — any type in the connection's catalog. */
export default function ResourcePage() {
  const { connectionId = '', resourceKey = '' } = useParams<{ connectionId: string; resourceKey: string }>();
  // Same query as the connection sidebar, so this is served from its cache.
  const { resourceTypes } = useResourceTypes({ pluginID: PLUGIN_ID, route: { connection: connectionId } });
  const label =
    resourceTypes.data?.find((t) => t.type === resourceKey)?.displayName || resourceKey.split('::').pop() || resourceKey;

  // Remount per kind so filter, sort, page and selection never leak between tables.
  return <ResourceTable key={`${connectionId}/${resourceKey}`} connectionId={connectionId} resourceKey={resourceKey} label={label} />;
}
