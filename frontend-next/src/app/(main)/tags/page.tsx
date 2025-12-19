'use client';

import { useState, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Tag as TagIcon,
  Plus,
  Search,
  ArrowUpDown,
  Pencil,
  Trash2,
  Loader2,
  CheckCircle,
  XCircle,
  BarChart3,
  TrendingUp,
} from 'lucide-react';
import { useAllTags, useCreateTag, useUpdateTag, useDeleteTag } from '@/hooks';
import type { Tag } from '@/types';

type SortField = 'name' | 'usage' | 'created';
type SortOrder = 'asc' | 'desc';

// Predefined color palette for tags
const COLOR_PALETTE = [
  '#EF4444', // red
  '#F97316', // orange
  '#F59E0B', // amber
  '#EAB308', // yellow
  '#84CC16', // lime
  '#22C55E', // green
  '#10B981', // emerald
  '#14B8A6', // teal
  '#06B6D4', // cyan
  '#0EA5E9', // sky
  '#3B82F6', // blue
  '#6366F1', // indigo
  '#8B5CF6', // violet
  '#A855F7', // purple
  '#D946EF', // fuchsia
  '#EC4899', // pink
  '#F43F5E', // rose
  '#6B7280', // gray
];

function ColorPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (color: string) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div
          className="h-8 w-8 rounded-md border"
          style={{ backgroundColor: value }}
        />
        <Input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-28 font-mono text-sm"
          placeholder="#000000"
        />
      </div>
      <div className="grid grid-cols-9 gap-1">
        {COLOR_PALETTE.map((color) => (
          <button
            key={color}
            type="button"
            className={`h-6 w-6 rounded-md border-2 transition-transform hover:scale-110 ${
              value === color ? 'border-foreground' : 'border-transparent'
            }`}
            style={{ backgroundColor: color }}
            onClick={() => onChange(color)}
            aria-label={`Select color ${color}`}
          />
        ))}
      </div>
    </div>
  );
}

function TagsPageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Skeleton className="mb-2 h-8 w-48" />
          <Skeleton className="h-4 w-96" />
        </div>
        <Skeleton className="h-10 w-32" />
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
        {[1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-24" />
        ))}
      </div>
      <Card>
        <CardContent className="pt-6">
          <div className="flex gap-4">
            <Skeleton className="h-10 flex-1" />
            <Skeleton className="h-10 w-40" />
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="flex items-center gap-4">
                <Skeleton className="h-6 w-6 rounded-full" />
                <Skeleton className="h-4 flex-1" />
                <Skeleton className="h-6 w-16" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

interface TagFormData {
  name: string;
  color: string;
  description: string;
}

const initialFormData: TagFormData = {
  name: '',
  color: '#6B7280',
  description: '',
};

export default function TagsPage() {
  const router = useRouter();
  const { data: tags, isLoading, error, refetch } = useAllTags();
  const createTagMutation = useCreateTag();
  const updateTagMutation = useUpdateTag();
  const deleteTagMutation = useDeleteTag();

  // UI State
  const [searchQuery, setSearchQuery] = useState('');
  const [sortField, setSortField] = useState<SortField>('name');
  const [sortOrder, setSortOrder] = useState<SortOrder>('asc');

  // Modal State
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showEditDialog, setShowEditDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [selectedTag, setSelectedTag] = useState<Tag | null>(null);
  const [formData, setFormData] = useState<TagFormData>(initialFormData);

  // Computed stats
  const stats = useMemo(() => {
    if (!tags) return { total: 0, used: 0, unused: 0, totalUsage: 0, avgUsage: '0' };
    const total = tags.length;
    const used = tags.filter((t) => (t.game_count ?? 0) > 0).length;
    const unused = total - used;
    const totalUsage = tags.reduce((sum, t) => sum + (t.game_count ?? 0), 0);
    const avgUsage = total > 0 ? (totalUsage / total).toFixed(1) : '0';
    return { total, used, unused, totalUsage, avgUsage };
  }, [tags]);

  // Filtered and sorted tags
  const filteredTags = useMemo(() => {
    if (!tags) return [];
    let result = [...tags];

    // Filter by search
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(
        (tag) =>
          tag.name.toLowerCase().includes(query) ||
          tag.description?.toLowerCase().includes(query)
      );
    }

    // Sort
    result.sort((a, b) => {
      let comparison = 0;
      switch (sortField) {
        case 'name':
          comparison = a.name.localeCompare(b.name);
          break;
        case 'usage':
          comparison = (a.game_count ?? 0) - (b.game_count ?? 0);
          break;
        case 'created':
          comparison = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
          break;
      }
      return sortOrder === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [tags, searchQuery, sortField, sortOrder]);

  // Suggest a color not yet used
  const suggestColor = () => {
    if (!tags) return COLOR_PALETTE[0];
    const usedColors = new Set(tags.map((t) => t.color));
    return COLOR_PALETTE.find((c) => !usedColors.has(c)) ?? COLOR_PALETTE[0];
  };

  // Handlers
  const handleTagClick = (tag: Tag) => {
    router.push(`/games?tag=${tag.id}`);
  };

  const handleOpenCreate = () => {
    setFormData({ ...initialFormData, color: suggestColor() });
    setShowCreateDialog(true);
  };

  const handleOpenEdit = (tag: Tag) => {
    setSelectedTag(tag);
    setFormData({
      name: tag.name,
      color: tag.color,
      description: tag.description ?? '',
    });
    setShowEditDialog(true);
  };

  const handleOpenDelete = (tag: Tag) => {
    setSelectedTag(tag);
    setShowDeleteDialog(true);
  };

  const handleCreate = async () => {
    if (!formData.name.trim()) {
      toast.error('Tag name is required');
      return;
    }

    try {
      await createTagMutation.mutateAsync({
        name: formData.name.trim(),
        color: formData.color,
        description: formData.description.trim() || undefined,
      });
      toast.success('Tag created successfully');
      setShowCreateDialog(false);
      setFormData(initialFormData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create tag';
      toast.error(message);
    }
  };

  const handleUpdate = async () => {
    if (!selectedTag || !formData.name.trim()) {
      toast.error('Tag name is required');
      return;
    }

    try {
      await updateTagMutation.mutateAsync({
        id: selectedTag.id,
        data: {
          name: formData.name.trim(),
          color: formData.color,
          description: formData.description.trim() || undefined,
        },
      });
      toast.success('Tag updated successfully');
      setShowEditDialog(false);
      setSelectedTag(null);
      setFormData(initialFormData);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update tag';
      toast.error(message);
    }
  };

  const handleDelete = async () => {
    if (!selectedTag) return;

    try {
      await deleteTagMutation.mutateAsync(selectedTag.id);
      toast.success('Tag deleted successfully');
      setShowDeleteDialog(false);
      setSelectedTag(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete tag';
      toast.error(message);
    }
  };

  const toggleSort = (field: SortField) => {
    if (sortField === field) {
      setSortOrder((prev) => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortField(field);
      setSortOrder('asc');
    }
  };

  if (isLoading) {
    return <TagsPageSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <XCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-semibold">Failed to load tags</h2>
        <p className="text-muted-foreground">{error.message}</p>
        <Button onClick={() => refetch()} className="mt-4">
          Try Again
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <TagIcon className="h-6 w-6" />
            Tag Management
          </h1>
          <p className="text-muted-foreground">
            Organize your games with custom tags. Click any tag to see games with that tag.
          </p>
        </div>
        <Button onClick={handleOpenCreate}>
          <Plus className="mr-2 h-4 w-4" />
          Create Tag
        </Button>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <TagIcon className="h-5 w-5 text-muted-foreground" />
              <div>
                <p className="text-sm text-muted-foreground">Total Tags</p>
                <p className="text-2xl font-bold">{stats.total}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <CheckCircle className="h-5 w-5 text-green-500" />
              <div>
                <p className="text-sm text-muted-foreground">Used Tags</p>
                <p className="text-2xl font-bold">{stats.used}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <XCircle className="h-5 w-5 text-muted-foreground" />
              <div>
                <p className="text-sm text-muted-foreground">Unused Tags</p>
                <p className="text-2xl font-bold">{stats.unused}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <BarChart3 className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm text-muted-foreground">Total Usage</p>
                <p className="text-2xl font-bold">{stats.totalUsage}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <TrendingUp className="h-5 w-5 text-purple-500" />
              <div>
                <p className="text-sm text-muted-foreground">Avg per Tag</p>
                <p className="text-2xl font-bold">{stats.avgUsage}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Search and Sort Controls */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search tags..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Sort by:</span>
              <div className="flex rounded-md border">
                <Button
                  variant={sortField === 'name' ? 'secondary' : 'ghost'}
                  size="sm"
                  className="rounded-r-none"
                  onClick={() => toggleSort('name')}
                >
                  Name
                  {sortField === 'name' && (
                    <ArrowUpDown className="ml-1 h-3 w-3" />
                  )}
                </Button>
                <Button
                  variant={sortField === 'usage' ? 'secondary' : 'ghost'}
                  size="sm"
                  className="rounded-none border-x"
                  onClick={() => toggleSort('usage')}
                >
                  Usage
                  {sortField === 'usage' && (
                    <ArrowUpDown className="ml-1 h-3 w-3" />
                  )}
                </Button>
                <Button
                  variant={sortField === 'created' ? 'secondary' : 'ghost'}
                  size="sm"
                  className="rounded-l-none"
                  onClick={() => toggleSort('created')}
                >
                  Date
                  {sortField === 'created' && (
                    <ArrowUpDown className="ml-1 h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Tags List */}
      <Card>
        <CardHeader>
          <CardTitle>Tags ({filteredTags.length})</CardTitle>
          <CardDescription>Click a tag to view games with that tag</CardDescription>
        </CardHeader>
        <CardContent>
          {filteredTags.length === 0 ? (
            <div className="py-12 text-center">
              {searchQuery ? (
                <>
                  <Search className="mx-auto h-12 w-12 text-muted-foreground" />
                  <h3 className="mt-2 font-medium">No tags found</h3>
                  <p className="text-sm text-muted-foreground">
                    No tags match your search &quot;{searchQuery}&quot;
                  </p>
                </>
              ) : (
                <>
                  <TagIcon className="mx-auto h-12 w-12 text-muted-foreground" />
                  <h3 className="mt-2 font-medium">No tags</h3>
                  <p className="text-sm text-muted-foreground">
                    Get started by creating your first tag to organize your games.
                  </p>
                  <Button onClick={handleOpenCreate} className="mt-4">
                    Create Your First Tag
                  </Button>
                </>
              )}
            </div>
          ) : (
            <div className="divide-y">
              {filteredTags.map((tag) => (
                <div
                  key={tag.id}
                  className="flex items-center justify-between py-4 transition-colors hover:bg-muted/50"
                >
                  <button
                    type="button"
                    className="flex flex-1 items-center gap-4 text-left"
                    onClick={() => handleTagClick(tag)}
                  >
                    <div
                      className="h-6 w-6 shrink-0 rounded-full border"
                      style={{ backgroundColor: tag.color }}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{tag.name}</span>
                        {(tag.game_count ?? 0) > 0 ? (
                          <span className="rounded-full bg-muted px-2 py-0.5 text-xs">
                            {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
                          </span>
                        ) : (
                          <span className="rounded-full bg-muted/50 px-2 py-0.5 text-xs text-muted-foreground">
                            Unused
                          </span>
                        )}
                      </div>
                      {tag.description && (
                        <p className="truncate text-sm text-muted-foreground">
                          {tag.description}
                        </p>
                      )}
                      <p className="text-xs text-muted-foreground">
                        Created {new Date(tag.created_at).toLocaleDateString()}
                      </p>
                    </div>
                  </button>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleOpenEdit(tag)}
                    >
                      <Pencil className="h-4 w-4" />
                      <span className="sr-only">Edit</span>
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleOpenDelete(tag)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                      <span className="sr-only">Delete</span>
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create Tag Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create New Tag</DialogTitle>
            <DialogDescription>
              Create a new tag to organize your games.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="tag-name">Name *</Label>
              <Input
                id="tag-name"
                value={formData.name}
                onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="Enter tag name..."
                maxLength={100}
              />
            </div>
            <div className="space-y-2">
              <Label>Color</Label>
              <ColorPicker
                value={formData.color}
                onChange={(color) => setFormData((prev) => ({ ...prev, color }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="tag-description">Description</Label>
              <Input
                id="tag-description"
                value={formData.description}
                onChange={(e) =>
                  setFormData((prev) => ({ ...prev, description: e.target.value }))
                }
                placeholder="Optional description..."
                maxLength={500}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={createTagMutation.isPending}>
              {createTagMutation.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Create Tag
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Tag Dialog */}
      <Dialog open={showEditDialog} onOpenChange={setShowEditDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Tag</DialogTitle>
            <DialogDescription>Update the tag details.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="edit-tag-name">Name *</Label>
              <Input
                id="edit-tag-name"
                value={formData.name}
                onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="Enter tag name..."
                maxLength={100}
              />
            </div>
            <div className="space-y-2">
              <Label>Color</Label>
              <ColorPicker
                value={formData.color}
                onChange={(color) => setFormData((prev) => ({ ...prev, color }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-tag-description">Description</Label>
              <Input
                id="edit-tag-description"
                value={formData.description}
                onChange={(e) =>
                  setFormData((prev) => ({ ...prev, description: e.target.value }))
                }
                placeholder="Optional description..."
                maxLength={500}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowEditDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleUpdate} disabled={updateTagMutation.isPending}>
              {updateTagMutation.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Update Tag
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Tag</AlertDialogTitle>
            <AlertDialogDescription>
              {selectedTag && (selectedTag.game_count ?? 0) > 0
                ? `Delete "${selectedTag.name}"? This will remove it from ${selectedTag.game_count} game${selectedTag.game_count !== 1 ? 's' : ''}.`
                : `Delete "${selectedTag?.name}"?`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteTagMutation.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
