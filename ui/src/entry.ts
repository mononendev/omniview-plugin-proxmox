import {
  PluginWindow,
  RESOURCE_ICON_EXTENSION_POINT_ID,
  type ExtensionRegistration,
  type ResourceIconSetV1,
  type SearchRouteHandleV1,
} from '@omniviewdev/runtime';
import type { RouteObject } from 'react-router-dom';

import { PLUGIN_ID } from './constants';
import { proxmoxResourceIconSet } from './icons/resourceIcons';
import ConnectionLayout from './pages/ConnectionLayout';
import ConnectionsPage from './pages/ConnectionsPage';
import ResourcePage, { ConnectionIndexRedirect } from './pages/ResourcePage';

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
      { path: 'resources/:resourceKey', Component: ResourcePage },
    ],
  },
];

export const extensionRegistrations: ExtensionRegistration<ResourceIconSetV1>[] = [
  {
    extensionPointId: RESOURCE_ICON_EXTENSION_POINT_ID,
    registration: {
      id: 'resource-icons',
      plugin: PLUGIN_ID,
      label: 'Proxmox VE resource icons',
      value: proxmoxResourceIconSet,
    },
  },
];

export const plugin = new PluginWindow().setRootPage(ConnectionsPage).withRoutes(routes);
