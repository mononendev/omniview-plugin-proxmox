import { useState } from 'react';
import {
  CodeBlock,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DescriptionList,
  Stack,
  StatusDot,
  Text,
} from '@omniviewdev/base-ui';
import { LuChevronDown, LuChevronRight } from 'react-icons/lu';

import { type Column, type NativeObject, cellValue, columnStatus, formatValue } from '../utils/columns';
import { REFS_FIELD } from '../utils/refs';

const sectionTitleStyle = { margin: '4px 0' } as const;
const triggerStyle = { display: 'inline-flex', alignItems: 'center', gap: 4 } as const;

function isScalar(value: unknown): boolean {
  return typeof value !== 'object' || value === null;
}

/**
 * A nested object, or a list holding any, collapsed to its size. The
 * formatted JSON mounts only when opened: Ceph maps run to hundreds of KiB.
 */
function NestedValue({ value }: { value: object }) {
  const [open, setOpen] = useState(false);
  const size = Array.isArray(value) ? value.length : Object.keys(value).length;
  const noun = Array.isArray(value) ? (size === 1 ? 'item' : 'items') : size === 1 ? 'field' : 'fields';
  return (
    <Collapsible open={open} onOpenChange={setOpen} animation="none">
      <CollapsibleTrigger style={triggerStyle}>
        {open ? <LuChevronDown size={12} /> : <LuChevronRight size={12} />}
        <Text size="sm" tone="muted">
          {size} {noun}
        </Text>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {open && <CodeBlock language="json" code={JSON.stringify(value, null, 2)} wrap maxHeight={320} />}
      </CollapsibleContent>
    </Collapsible>
  );
}

/** An object or list field: lists of plain values inline, anything else collapsed. */
function ObjectValue({ value }: { value: object }) {
  if (Object.keys(value).length === 0) return '—';
  if (Array.isArray(value) && value.every(isScalar)) return value.map(String).join(', ');
  return <NestedValue value={value} />;
}

type Props = {
  data?: NativeObject;
  columns: Column[];
};

/**
 * Generic detail view: the kind's table columns first (formatted the same
 * way as the table), then every other top-level field of the native object,
 * nested objects collapsed. The plugin's reference object is left out; the
 * Related view shows it.
 */
export default function ResourceOverview({ data, columns }: Props) {
  if (!data) return null;

  const covered = new Set([...columns.map((col) => col.field), REFS_FIELD]);
  const otherFields = Object.keys(data)
    .filter((field) => !covered.has(field))
    .sort();

  return (
    <Stack direction="column" spacing={3} style={{ width: '100%' }}>
      <section>
        <Text weight="semibold" size="sm" style={sectionTitleStyle}>
          Summary
        </Text>
        <DescriptionList layout="horizontal" size="sm">
          {columns.map((col) => {
            const value = cellValue(data, col);
            const status = columnStatus(value, col);
            const text = formatValue(value, col);
            return (
              <DescriptionList.Item key={col.field} label={col.header}>
                {status ? <StatusDot status={status} label={text} size="sm" /> : text || '—'}
              </DescriptionList.Item>
            );
          })}
        </DescriptionList>
      </section>
      {otherFields.length > 0 && (
        <section>
          <Text weight="semibold" size="sm" style={sectionTitleStyle}>
            All fields
          </Text>
          <DescriptionList layout="horizontal" size="sm">
            {otherFields.map((field) => {
              const value = data[field];
              return (
                <DescriptionList.Item key={field} label={field} copyable={isScalar(value)}>
                  {isScalar(value) ? String(value ?? '') : <ObjectValue value={value as object} />}
                </DescriptionList.Item>
              );
            })}
          </DescriptionList>
        </section>
      )}
    </Stack>
  );
}
