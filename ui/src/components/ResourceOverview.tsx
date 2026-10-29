import { DescriptionList, Stack, StatusDot, Text } from '@omniviewdev/base-ui';

import { type Column, type NativeObject, cellValue, columnStatus, formatValue } from '../utils/columns';
import { REFS_FIELD } from '../utils/refs';

const sectionTitleStyle = { margin: '4px 0' } as const;

type Props = {
  data?: NativeObject;
  columns: Column[];
};

/**
 * Generic detail view: the kind's table columns first (formatted the same
 * way as the table), then every other top-level field of the native object.
 * The plugin's reference object is left out; the Related view shows it.
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
                <DescriptionList.Item key={field} label={field} copyable>
                  {typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value ?? '')}
                </DescriptionList.Item>
              );
            })}
          </DescriptionList>
        </section>
      )}
    </Stack>
  );
}
