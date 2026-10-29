import { useCallback } from 'react';
import { CodeBlock } from '@omniviewdev/base-ui';
import { ResourceIcon, useRightDrawer, type DrawerComponent } from '@omniviewdev/runtime';
import { LuBraces, LuSquareChartGantt, LuWaypoints } from 'react-icons/lu';

import CephStatusOverview from '../components/CephStatusOverview';
import RelatedResources from '../components/RelatedResources';
import ResourceOverview from '../components/ResourceOverview';
import { GRAPH_RESOURCE_KEYS, PLUGIN_ID, RESOURCE_KEYS } from '../constants';
import { type NativeObject, displayName, kindFor, objectId } from '../utils/columns';

/**
 * Opens the detail drawer of one object: overview, related resources (for
 * kinds in the relationship graph) and raw JSON.
 */
export function useOpenResource(connectionId: string) {
  const { openDrawer } = useRightDrawer();

  return useCallback(
    function open(resourceKey: string, row: NativeObject) {
      const kind = kindFor(resourceKey);
      const id = objectId(row, kind);
      const name = displayName(row, kind);
      const views: DrawerComponent<NativeObject>['views'] = [
        {
          title: 'Overview',
          icon: <LuSquareChartGantt />,
          component: (ctx) =>
            resourceKey === RESOURCE_KEYS.cephCluster ? (
              <CephStatusOverview data={ctx.data} />
            ) : (
              <ResourceOverview data={ctx.data} columns={kind.columns} />
            ),
        },
      ];
      if (GRAPH_RESOURCE_KEYS.has(resourceKey)) {
        views.push({
          title: 'Related',
          icon: <LuWaypoints />,
          component: () => (
            <RelatedResources connectionId={connectionId} resourceKey={resourceKey} resourceId={id} onOpen={open} />
          ),
        });
      }
      views.push({
        title: 'JSON',
        icon: <LuBraces />,
        component: (ctx) => <CodeBlock language="json" code={JSON.stringify(ctx.data, null, 2)} copyable wrap />,
      });
      const drawer: DrawerComponent<NativeObject> = {
        title: name,
        icon: <ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={16} />,
        views,
        actions: [],
      };
      openDrawer(drawer, {
        data: row,
        resource: { id, name, key: resourceKey, connectionID: connectionId, pluginID: PLUGIN_ID },
      });
    },
    [openDrawer, connectionId],
  );
}
