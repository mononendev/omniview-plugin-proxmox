import {
  PluginWindow,
  RESOURCE_ICON_EXTENSION_POINT_ID,
  type ExtensionRegistration,
  type SearchRouteHandleV1,
} from '@omniviewdev/runtime';
import type { RouteObject } from 'react-router-dom';

import ProxmoxHomepageCard, { proxmoxHomepageCardMeta } from './components/homepage/ProxmoxHomepageCard';
import { PLUGIN_ID } from './constants';
import { proxmoxResourceIconSet } from './icons/resourceIcons';
import ConnectionLayout from './pages/ConnectionLayout';
import ConnectionsPage from './pages/ConnectionsPage';
import OverviewPage, { ConnectionIndexRedirect } from './pages/OverviewPage';
import ResourcePage from './pages/ResourcePage';
import TopologyPage from './pages/TopologyPage';

/** Seeds header search and Cmd/Ctrl-K with the connection being viewed. */
const connectionSearchHandle: SearchRouteHandleV1 = {
  searchScope: ({ pluginId, params }) =>
    params.connectionId
      ? { kind: 'connection', pluginId, connectionId: params.connectionId, label: params.connectionId }
      : undefined,
};

const routes: RouteObject[] = [
  { path: '/', Component: ConnectionsPage },
  {
    path: '/connection/:connectionId',
    Component: ConnectionLayout,
    handle: connectionSearchHandle,
    children: [
      { index: true, Component: ConnectionIndexRedirect },
      { path: 'overview', Component: OverviewPage },
      { path: 'resources/:resourceKey', Component: ResourcePage },
      { path: 'topology', Component: TopologyPage },
    ],
  },
];

export const extensionRegistrations: ExtensionRegistration[] = [
  {
    extensionPointId: RESOURCE_ICON_EXTENSION_POINT_ID,
    registration: {
      id: 'resource-icons',
      plugin: PLUGIN_ID,
      label: 'Proxmox VE resource icons',
      value: proxmoxResourceIconSet,
    },
  },
  {
    extensionPointId: 'omniview/home/card',
    registration: {
      id: 'proxmox/home/card',
      plugin: PLUGIN_ID,
      label: 'Proxmox VE',
      value: ProxmoxHomepageCard,
      meta: proxmoxHomepageCardMeta,
    },
  },
];

export const plugin = new PluginWindow().setRootPage(ConnectionsPage).withRoutes(routes);
