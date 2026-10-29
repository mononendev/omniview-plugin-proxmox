import type { ComponentType } from 'react';
import type { IconType } from 'react-icons';
import {
  LuBox,
  LuCylinder,
  LuDatabase,
  LuEye,
  LuFileStack,
  LuFolderTree,
  LuGauge,
  LuHardDrive,
  LuHeartPulse,
  LuMonitor,
  LuServer,
} from 'react-icons/lu';
import type { ResourceIconProps, ResourceIconSetV1 } from '@omniviewdev/runtime';

import { RESOURCE_KEYS } from '../constants';

/** Lucide glyphs are stroked with currentColor, so they match adjacent monochrome controls. */
function glyph(Icon: IconType): ComponentType<ResourceIconProps> {
  return function ProxmoxResourceIcon({ size = 16, className, 'aria-hidden': hidden = true }) {
    return <Icon size={size} className={className} aria-hidden={hidden} focusable="false" style={{ flexShrink: 0 }} />;
  };
}

const icons: Record<string, ComponentType<ResourceIconProps>> = {
  [RESOURCE_KEYS.node]: glyph(LuServer),
  [RESOURCE_KEYS.storage]: glyph(LuHardDrive),
  [RESOURCE_KEYS.virtualMachine]: glyph(LuMonitor),
  [RESOURCE_KEYS.container]: glyph(LuBox),
  [RESOURCE_KEYS.cephCluster]: glyph(LuHeartPulse),
  [RESOURCE_KEYS.cephPool]: glyph(LuDatabase),
  [RESOURCE_KEYS.cephOSD]: glyph(LuCylinder),
  [RESOURCE_KEYS.cephMonitor]: glyph(LuEye),
  [RESOURCE_KEYS.cephManager]: glyph(LuGauge),
  [RESOURCE_KEYS.cephMetadataServer]: glyph(LuFileStack),
  [RESOURCE_KEYS.cephFilesystem]: glyph(LuFolderTree),
};

export const proxmoxResourceIconSet: ResourceIconSetV1 = {
  schemaVersion: 1,
  resolve: (resourceKey) => icons[resourceKey],
};
