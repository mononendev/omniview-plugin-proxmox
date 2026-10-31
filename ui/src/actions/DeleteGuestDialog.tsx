import { useState } from 'react';
import { Banner, Button, Dialog, FormField, Input, Text } from '@omniviewdev/base-ui';
import { LuTrash2 } from 'react-icons/lu';

import type { ActionTarget, MutateResource } from './actionDeps';

const bodyStyle = { display: 'flex', flexDirection: 'column', gap: 12 } as const;

type Props = {
  target: ActionTarget;
  mutate: MutateResource;
  onClose: () => void;
  /** Runs after the guest is gone. */
  onDeleted: () => void;
};

/**
 * Confirms destroying a guest the way the Proxmox web UI does: by typing its
 * VMID. The backend purges it from jobs and removes unreferenced disks.
 */
export default function DeleteGuestDialog({ target, mutate, onClose, onDeleted }: Props) {
  const vmid = String(target.data.vmid ?? '');
  const [typed, setTyped] = useState('');
  const [deleting, setDeleting] = useState(false);
  const id = 'proxmox-delete-confirm';

  const confirm = async () => {
    setDeleting(true);
    try {
      await mutate({
        route: target.record.route,
        reference: target.record.record.reference,
        operation: 'delete',
        input: { kind: 'map', value: {} },
      });
      onClose();
      onDeleted();
    } catch {
      // Reported by the runtime's notification; leave the dialog open to retry or cancel.
      setDeleting(false);
    }
  };

  return (
    <Dialog open onClose={onClose} size="sm">
      <Dialog.Close aria-label="Close" />
      <Dialog.Title icon={<LuTrash2 size={16} color="var(--ov-color-danger)" />}>Destroy {target.name}?</Dialog.Title>
      <Dialog.Body>
        <div style={bodyStyle}>
          <Banner color="danger" size="sm">
            <Banner.Content>
              This permanently removes <strong>{target.id}</strong>, all of its disks and its entries in backup,
              replication and HA jobs. It cannot be undone.
            </Banner.Content>
          </Banner>
          <FormField label={`Type the VMID ${vmid} to confirm`} htmlFor={id} size="sm">
            <Input.Control
              id={id}
              size="sm"
              mono
              autoComplete="off"
              value={typed}
              disabled={deleting}
              onChange={(e) => setTyped(e.target.value)}
            />
          </FormField>
          <Text size="xs" tone="muted">
            Proxmox refuses to destroy a running guest; stop it first.
          </Text>
        </div>
      </Dialog.Body>
      <Dialog.Footer>
        <Button size="sm" variant="outline" color="neutral" onClick={onClose}>
          Cancel
        </Button>
        <Button
          size="sm"
          variant="solid"
          color="danger"
          disabled={!vmid || typed.trim() !== vmid || deleting}
          onClick={() => void confirm()}
        >
          {deleting ? 'Destroying…' : 'Destroy'}
        </Button>
      </Dialog.Footer>
    </Dialog>
  );
}
