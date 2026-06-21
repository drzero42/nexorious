import { PlayStatus, OwnershipStatus } from '@/types';

export const playStatusOptions: { value: PlayStatus; label: string }[] = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.DROPPED, label: 'Dropped' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

export const ownershipStatusOptions: { value: OwnershipStatus; label: string }[] = [
  { value: OwnershipStatus.OWNED, label: 'Owned' },
  { value: OwnershipStatus.BORROWED, label: 'Borrowed' },
  { value: OwnershipStatus.RENTED, label: 'Rented' },
  { value: OwnershipStatus.SUBSCRIPTION, label: 'Subscription' },
  { value: OwnershipStatus.NO_LONGER_OWNED, label: 'No Longer Owned' },
];

export const playStatusLabels: Record<string, string> = Object.fromEntries(
  playStatusOptions.map((o) => [o.value, o.label]),
);

export const ownershipStatusLabels: Record<string, string> = Object.fromEntries(
  ownershipStatusOptions.map((o) => [o.value, o.label]),
);
