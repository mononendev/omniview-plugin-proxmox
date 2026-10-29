import { useCallback } from 'react';
import { CodeBlock } from '@omniviewdev/base-ui';
import { ResourceIcon, useRightDrawer, type DrawerComponent } from '@omniviewdev/runtime';
import { LuBraces, LuChartLine, LuServer, LuSquareChartGantt, LuWaypoints } from 'react-icons/lu';

import DrawerActionsBridge from '../actions/DrawerActionsBridge';
import { resourceActions } from '../actions/resourceActions';
import CephStatusOverview from '../components/CephStatusOverview';
import NodeGuests from '../components/NodeGuests';
import RelatedResources from '../components/RelatedResources';
import ResourceMetrics from '../components/ResourceMetrics';
import ResourceOverview from '../components/ResourceOverview';
import { GRAPH_RESOURCE_KEYS, METRIC_RESOURCE_KEYS, PLUGIN_ID, RESOURCE_KEYS } from '../constants';
import { type NativeObject, displayName, kindFor, objectId } from '../utils/columns';

/**
 * Opens the detail drawer of one object: overview, related resources (for
 * kinds in the relationship graph), metrics, a node's guests and raw JSON,
 * with the kind's actions in the header.
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
      if (METRIC_RESOURCE_KEYS.has(resourceKey)) {
        views.push({
          title: 'Metrics',
          icon: <LuChartLine />,
          component: (ctx) => (
            <ResourceMetrics connectionId={connectionId} resourceKey={resourceKey} resourceId={id} data={ctx.data} />
          ),
        });
      }
      if (resourceKey === RESOURCE_KEYS.node) {
        views.push({
          title: 'Guests',
          icon: <LuServer />,
          component: () => <NodeGuests connectionId={connectionId} node={String(row.node ?? '')} onOpen={open} />,
        });
      }
      views.push({
        title: 'JSON',
        icon: <LuBraces />,
        component: (ctx) => <CodeBlock language="json" code={JSON.stringify(ctx.data, null, 2)} copyable wrap />,
      });
      // Status-dependent actions follow the object while the drawer is open.
      const live = METRIC_RESOURCE_KEYS.has(resourceKey);
      const drawer: DrawerComponent<NativeObject> = {
        title: name,
        icon: <ResourceIcon pluginId={PLUGIN_ID} resourceKey={resourceKey} size={16} />,
        views: views.map((view) => ({
          ...view,
          component: (ctx) => (
            <DrawerActionsBridge ctx={ctx} drawer={drawer} live={live}>
              {view.component(ctx)}
            </DrawerActionsBridge>
          ),
        })),
        actions: resourceActions(resourceKey),
      };
      openDrawer(drawer, {
        data: row,
        resource: { id, name, key: resourceKey, connectionID: connectionId, pluginID: PLUGIN_ID },
      });
    },
    [openDrawer, connectionId],
  );
}
