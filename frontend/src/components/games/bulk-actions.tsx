'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useBulkUpdateUserGames, useBulkDeleteUserGames } from '@/hooks';
import { PlayStatus, SelectionMode } from '@/types';
import { Trash2, X } from 'lucide-react';

export interface BulkActionsProps {
  selectedIds: Set<string>;
  onClearSelection: () => void;
  onSuccess?: () => void;
  // New props for select all
  selectionMode: SelectionMode;
  visibleCount: number;
  totalCount: number;
  onSelectAllClick: () => void;
}

const statusOptions: { value: PlayStatus; label: string }[] = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

export function BulkActions({
  selectedIds,
  onClearSelection,
  onSuccess,
  selectionMode,
  visibleCount,
  totalCount,
  onSelectAllClick,
}: BulkActionsProps) {
  const bulkUpdate = useBulkUpdateUserGames();
  const bulkDelete = useBulkDeleteUserGames();
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

  // Don't render if no games exist
  if (totalCount === 0) {
    return null;
  }

  const handleStatusChange = async (status: PlayStatus) => {
    try {
      await bulkUpdate.mutateAsync({
        ids: Array.from(selectedIds),
        updates: { playStatus: status },
      });
      onClearSelection();
      onSuccess?.();
    } catch (error) {
      console.error('Failed to update games:', error);
    }
  };

  const handleDelete = async () => {
    try {
      await bulkDelete.mutateAsync(Array.from(selectedIds));
      setIsDeleteDialogOpen(false);
      onClearSelection();
      onSuccess?.();
    } catch (error) {
      console.error('Failed to delete games:', error);
    }
  };

  const isLoading = bulkUpdate.isPending || bulkDelete.isPending;
  const selectedCount = selectedIds.size;
  const hasSelection = selectedCount > 0;

  // Determine checkbox state
  const isAllVisibleSelected = selectionMode === 'all-visible' ||
    (selectionMode === 'manual' && selectedCount === visibleCount && visibleCount > 0);
  const isAllCollectionSelected = selectionMode === 'all-collection';
  const isIndeterminate = selectionMode === 'manual' && selectedCount > 0 && selectedCount < visibleCount;
  const isChecked = isAllVisibleSelected || isAllCollectionSelected;

  // Determine label text
  const getSelectionLabel = (): string => {
    if (isAllCollectionSelected) {
      return `All ${totalCount} in library selected`;
    }
    if (isAllVisibleSelected) {
      // Check if visible = total (collapsed state)
      if (visibleCount === totalCount) {
        return `All ${totalCount} game${totalCount !== 1 ? 's' : ''} selected`;
      }
      return `All ${visibleCount} visible selected`;
    }
    return 'Select all';
  };

  return (
    <div className="flex items-center gap-4 p-3 bg-muted rounded-lg">
      {/* Select all checkbox */}
      <div className="flex items-center gap-2">
        <Checkbox
          id="select-all"
          checked={isIndeterminate ? 'indeterminate' : isChecked}
          onCheckedChange={() => onSelectAllClick()}
          disabled={isLoading}
        />
        <label
          htmlFor="select-all"
          className="text-sm font-medium cursor-pointer select-none"
        >
          {getSelectionLabel()}
        </label>
      </div>

      {/* Show actions only when games are selected */}
      {hasSelection && (
        <>
          <div className="h-4 w-px bg-border" />

          <span className="text-sm text-muted-foreground">
            {selectedCount} game{selectedCount !== 1 ? 's' : ''} selected
          </span>

          {/* Bulk status change */}
          <Select
            onValueChange={(value) => handleStatusChange(value as PlayStatus)}
            disabled={isLoading}
          >
            <SelectTrigger className="w-40">
              <SelectValue placeholder="Change status" />
            </SelectTrigger>
            <SelectContent>
              {statusOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Delete button with confirmation */}
          <AlertDialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" size="sm" disabled={isLoading}>
                <Trash2 className="h-4 w-4 mr-1" />
                Delete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete Games</AlertDialogTitle>
                <AlertDialogDescription>
                  Are you sure you want to delete {selectedCount} game
                  {selectedCount !== 1 ? 's' : ''}? This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={handleDelete} disabled={bulkDelete.isPending}>
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>

          {/* Clear selection */}
          <Button variant="ghost" size="sm" onClick={onClearSelection} disabled={isLoading}>
            <X className="h-4 w-4 mr-1" />
            Clear
          </Button>
        </>
      )}
    </div>
  );
}
