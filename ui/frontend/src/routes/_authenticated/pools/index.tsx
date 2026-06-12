import { useState } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
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
import { ListChecks, Plus, XCircle } from 'lucide-react';
import { usePools, useCreatePool, useUpdatePool, useDeletePool, useReorderPools } from '@/hooks';
import { PoolCard } from '@/components/pools/pool-card';
import { PoolFormDialog, type PoolFormValues } from '@/components/pools/pool-form-dialog';
import { reorderQueue } from '@/lib/pool-queue';
import type { PoolListItem } from '@/types';

export const Route = createFileRoute('/_authenticated/pools/')({
  head: () => ({ meta: [{ title: 'Planning | Nexorious' }] }),
  component: PoolsIndexPage,
});

function PoolsIndexPage() {
  const navigate = useNavigate();
  const { data: pools, isLoading, error, refetch } = usePools();
  const createPool = useCreatePool();
  const updatePool = useUpdatePool();
  const deletePool = useDeletePool();
  const reorderPools = useReorderPools();

  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<PoolListItem | null>(null);
  const [deleting, setDeleting] = useState<PoolListItem | null>(null);

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  const handleSubmit = async (values: PoolFormValues) => {
    try {
      if (editing) {
        await updatePool.mutateAsync({ id: editing.id, data: values });
        toast.success('Pool updated');
      } else {
        await createPool.mutateAsync(values);
        toast.success('Pool created');
      }
      setShowForm(false);
      setEditing(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save pool');
    }
  };

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deletePool.mutateAsync(deleting.id);
      toast.success('Pool deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete pool');
    }
    setDeleting(null);
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id || !pools) return;
    const from = pools.findIndex((p) => p.id === active.id);
    const to = pools.findIndex((p) => p.id === over.id);
    if (from < 0 || to < 0) return;
    const nextIds = reorderQueue(
      pools.map((p) => p.id),
      from,
      to,
    );
    reorderPools.mutate(nextIds, {
      onError: () => toast.error('Failed to reorder pools'),
    });
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <XCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-semibold">Failed to load pools</h2>
        <p className="text-muted-foreground">{error.message}</p>
        <Button onClick={() => refetch()} className="mt-4">
          Try Again
        </Button>
      </div>
    );
  }

  const list = pools ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <ListChecks className="h-6 w-6" />
            Planning
          </h1>
          <p className="text-muted-foreground">
            Group games into pools and line up what to play next.
          </p>
        </div>
        <Button
          onClick={() => {
            setEditing(null);
            setShowForm(true);
          }}
        >
          <Plus className="mr-2 h-4 w-4" />
          Create Pool
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Pools ({list.length})</CardTitle>
          <CardDescription>Drag to reorder. Click a pool to plan it.</CardDescription>
        </CardHeader>
        <CardContent>
          {list.length === 0 ? (
            <div className="py-12 text-center">
              <ListChecks className="mx-auto h-12 w-12 text-muted-foreground" />
              <h3 className="mt-2 font-medium">No pools yet</h3>
              <p className="text-sm text-muted-foreground">Create one to start planning.</p>
            </div>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext items={list.map((p) => p.id)} strategy={verticalListSortingStrategy}>
                <div>
                  {list.map((pool) => (
                    <PoolCard
                      key={pool.id}
                      pool={pool}
                      onOpen={(id) => navigate({ to: '/pools/$id', params: { id } })}
                      onEdit={(p) => {
                        setEditing(p);
                        setShowForm(true);
                      }}
                      onDelete={setDeleting}
                    />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          )}
        </CardContent>
      </Card>

      <PoolFormDialog
        open={showForm}
        onOpenChange={setShowForm}
        editing={editing}
        onSubmit={handleSubmit}
        pending={createPool.isPending || updatePool.isPending}
      />

      <AlertDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Pool</AlertDialogTitle>
            <AlertDialogDescription>
              Delete &quot;{deleting?.name}&quot;? This removes the pool and its membership; your
              games are not deleted.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
