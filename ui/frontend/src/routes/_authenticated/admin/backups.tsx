import { useState, useEffect, useCallback } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
import { Checkbox } from '@/components/ui/checkbox';
import { toast } from 'sonner';
import {
  DatabaseBackup,
  Download,
  Trash2,
  RotateCcw,
  Upload,
  Loader2,
  Settings,
  Clock,
  HardDrive,
  Users,
  Gamepad2,
  Tag,
  AlertTriangle,
} from 'lucide-react';
import * as backupApi from '@/api/backup';
import { triggerBlobDownload } from '@/api/import-export';
import type { BackupConfig, BackupInfo, BackupSchedule, RetentionMode } from '@/types';

export const Route = createFileRoute('/_authenticated/admin/backups')({
  component: BackupPage,
});

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString();
}

function getBackupTypeBadge(type: string) {
  switch (type) {
    case 'scheduled':
      return <Badge variant="default">Scheduled</Badge>;
    case 'manual':
      return <Badge variant="secondary">Manual</Badge>;
    case 'pre_restore':
      return <Badge variant="outline" className="border-orange-500 text-orange-600">Pre-restore</Badge>;
    default:
      return <Badge variant="outline">{type}</Badge>;
  }
}

function BackupPageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
      </div>
      <Skeleton className="h-96" />
    </div>
  );
}

function BackupPage() {
  const navigate = useNavigate();
  const { user: currentUser } = useAuth();
  const [isLoading, setIsLoading] = useState(true);
  const [config, setConfig] = useState<BackupConfig | null>(null);
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [isCreatingBackup, setIsCreatingBackup] = useState(false);
  const [isSavingConfig, setIsSavingConfig] = useState(false);
  const [isDownloading, setIsDownloading] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState<string | null>(null);
  const [isRestoring, setIsRestoring] = useState(false);

  // Config form state
  const [schedule, setSchedule] = useState<BackupSchedule>('manual');
  const [scheduleTime, setScheduleTime] = useState('02:00');
  const [scheduleDay, setScheduleDay] = useState<number>(0);
  const [retentionMode, setRetentionMode] = useState<RetentionMode>('count');
  const [retentionValue, setRetentionValue] = useState(5);

  // Dialog state
  const [restoreBackupId, setRestoreBackupId] = useState<string | null>(null);
  const [restoreConfirmed, setRestoreConfirmed] = useState(false);
  const [deleteBackupId, setDeleteBackupId] = useState<string | null>(null);
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadConfirmed, setUploadConfirmed] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [configData, backupsData] = await Promise.all([
        backupApi.getBackupConfig(),
        backupApi.listBackups(),
      ]);
      setConfig(configData);
      setBackups(backupsData);

      // Initialize form with config values
      setSchedule(configData.schedule);
      setScheduleTime(configData.scheduleTime);
      setScheduleDay(configData.scheduleDay ?? 0);
      setRetentionMode(configData.retentionMode);
      setRetentionValue(configData.retentionValue);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load backup data';
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      navigate({ to: '/dashboard', replace: true });
    } else if (currentUser?.isAdmin) {
      loadData();
    }
  }, [currentUser, navigate, loadData]);

  const handleSaveConfig = async () => {
    try {
      setIsSavingConfig(true);
      const updated = await backupApi.updateBackupConfig({
        schedule,
        schedule_time: scheduleTime,
        schedule_day: schedule === 'weekly' ? scheduleDay : undefined,
        retention_mode: retentionMode,
        retention_value: retentionValue,
      });
      setConfig(updated);
      toast.success('Backup configuration saved');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save configuration';
      toast.error(message);
    } finally {
      setIsSavingConfig(false);
    }
  };

  const handleCreateBackup = async () => {
    try {
      setIsCreatingBackup(true);
      const result = await backupApi.createBackup();
      toast.success(result.message);
      // Reload backups list after a short delay
      setTimeout(() => loadData(), 2000);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create backup';
      toast.error(message);
    } finally {
      setIsCreatingBackup(false);
    }
  };

  const handleDownload = async (backupId: string) => {
    try {
      setIsDownloading(backupId);
      const { blob, filename } = await backupApi.downloadBackup(backupId);
      triggerBlobDownload(blob, filename);
      toast.success('Backup downloaded');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to download backup';
      toast.error(message);
    } finally {
      setIsDownloading(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteBackupId) return;
    try {
      setIsDeleting(deleteBackupId);
      await backupApi.deleteBackup(deleteBackupId);
      toast.success('Backup deleted');
      setBackups(backups.filter(b => b.id !== deleteBackupId));
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete backup';
      toast.error(message);
    } finally {
      setIsDeleting(null);
      setDeleteBackupId(null);
    }
  };

  const handleRestore = async () => {
    if (!restoreBackupId) return;
    try {
      setIsRestoring(true);
      const result = await backupApi.restoreBackup(restoreBackupId);
      if (result.session_invalidated) {
        toast.success('Restore completed. You will be logged out.');
        setTimeout(() => navigate({ to: '/login' }), 2000);
      } else {
        toast.success(result.message);
        loadData();
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to restore backup';
      toast.error(message);
    } finally {
      setIsRestoring(false);
      setRestoreBackupId(null);
      setRestoreConfirmed(false);
    }
  };

  const handleUploadRestore = async () => {
    if (!uploadFile) return;
    try {
      setIsRestoring(true);
      const result = await backupApi.uploadAndRestoreBackup(uploadFile);
      if (result.session_invalidated) {
        toast.success('Restore completed. You will be logged out.');
        setTimeout(() => navigate({ to: '/login' }), 2000);
      } else {
        toast.success(result.message);
        loadData();
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to restore from upload';
      toast.error(message);
    } finally {
      setIsRestoring(false);
      setUploadDialogOpen(false);
      setUploadFile(null);
      setUploadConfirmed(false);
    }
  };

  // Show nothing while checking auth
  if (!currentUser?.isAdmin) {
    return null;
  }

  if (isLoading) {
    return <BackupPageSkeleton />;
  }

  const restoreBackup = restoreBackupId ? backups.find(b => b.id === restoreBackupId) : null;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="border-b pb-5">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link to="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link to="/admin" className="hover:text-foreground">
            Admin
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Backup / Restore</span>
        </nav>
        <h1 className="text-3xl font-bold">Backup / Restore</h1>
        <p className="mt-2 text-muted-foreground">
          Full system backup and restore for disaster recovery and migration
        </p>
      </div>

      {/* Configuration and Actions */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Configuration */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings className="h-5 w-5" />
              Backup Schedule
            </CardTitle>
            <CardDescription>Configure automatic backup schedule and retention</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>Schedule</Label>
              <Select value={schedule} onValueChange={(v) => setSchedule(v as BackupSchedule)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="manual">Manual only</SelectItem>
                  <SelectItem value="daily">Daily</SelectItem>
                  <SelectItem value="weekly">Weekly</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {schedule !== 'manual' && (
              <div className="space-y-2">
                <Label>Time (UTC)</Label>
                <Input
                  type="time"
                  value={scheduleTime}
                  onChange={(e) => setScheduleTime(e.target.value)}
                />
              </div>
            )}

            {schedule === 'weekly' && (
              <div className="space-y-2">
                <Label>Day of Week</Label>
                <Select value={String(scheduleDay)} onValueChange={(v) => setScheduleDay(Number(v))}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">Monday</SelectItem>
                    <SelectItem value="1">Tuesday</SelectItem>
                    <SelectItem value="2">Wednesday</SelectItem>
                    <SelectItem value="3">Thursday</SelectItem>
                    <SelectItem value="4">Friday</SelectItem>
                    <SelectItem value="5">Saturday</SelectItem>
                    <SelectItem value="6">Sunday</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}

            <div className="space-y-2">
              <Label>Retention Policy</Label>
              <div className="flex gap-2">
                <Select value={retentionMode} onValueChange={(v) => setRetentionMode(v as RetentionMode)}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="count">Keep last</SelectItem>
                    <SelectItem value="days">Keep for</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  type="number"
                  min={1}
                  value={retentionValue}
                  onChange={(e) => setRetentionValue(Number(e.target.value))}
                  className="w-20"
                />
                <span className="flex items-center text-sm text-muted-foreground">
                  {retentionMode === 'count' ? 'backups' : 'days'}
                </span>
              </div>
            </div>

            <Button onClick={handleSaveConfig} disabled={isSavingConfig} className="w-full">
              {isSavingConfig ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                'Save Configuration'
              )}
            </Button>
          </CardContent>
        </Card>

        {/* Actions */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <DatabaseBackup className="h-5 w-5" />
              Backup Actions
            </CardTitle>
            <CardDescription>Create backups or restore from existing ones</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Create Backup Now</p>
                  <p className="text-sm text-muted-foreground">
                    Full database and file backup
                  </p>
                </div>
                <Button onClick={handleCreateBackup} disabled={isCreatingBackup}>
                  {isCreatingBackup ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Creating...
                    </>
                  ) : (
                    <>
                      <DatabaseBackup className="mr-2 h-4 w-4" />
                      Backup Now
                    </>
                  )}
                </Button>
              </div>
            </div>

            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Restore from File</p>
                  <p className="text-sm text-muted-foreground">
                    Upload a backup archive to restore
                  </p>
                </div>
                <Button variant="outline" onClick={() => setUploadDialogOpen(true)}>
                  <Upload className="mr-2 h-4 w-4" />
                  Upload & Restore
                </Button>
              </div>
            </div>

            {config && (
              <div className="rounded-lg bg-muted/50 p-4 text-sm">
                <div className="flex items-center gap-2 text-muted-foreground">
                  <Clock className="h-4 w-4" />
                  <span>
                    {config.schedule === 'manual'
                      ? 'No automatic backups scheduled'
                      : config.schedule === 'daily'
                        ? `Daily at ${config.scheduleTime} UTC`
                        : `Weekly on ${['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][config.scheduleDay ?? 0]} at ${config.scheduleTime} UTC`}
                  </span>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Backups List */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HardDrive className="h-5 w-5" />
            Available Backups
          </CardTitle>
          <CardDescription>
            {backups.length} backup{backups.length !== 1 ? 's' : ''} available
          </CardDescription>
        </CardHeader>
        <CardContent>
          {backups.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-center text-muted-foreground">
              <DatabaseBackup className="mb-4 h-12 w-12 opacity-50" />
              <p>No backups yet</p>
              <p className="text-sm">Create your first backup to protect your data</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Created</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Contents</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {backups.map((backup) => (
                  <TableRow key={backup.id}>
                    <TableCell className="font-medium">
                      {formatDate(backup.createdAt)}
                    </TableCell>
                    <TableCell>{getBackupTypeBadge(backup.backupType)}</TableCell>
                    <TableCell>{formatBytes(backup.sizeBytes)}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-3 text-sm text-muted-foreground">
                        <span className="flex items-center gap-1">
                          <Users className="h-3 w-3" />
                          {backup.stats.users}
                        </span>
                        <span className="flex items-center gap-1">
                          <Gamepad2 className="h-3 w-3" />
                          {backup.stats.games}
                        </span>
                        <span className="flex items-center gap-1">
                          <Tag className="h-3 w-3" />
                          {backup.stats.tags}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleDownload(backup.id)}
                          disabled={isDownloading === backup.id}
                        >
                          {isDownloading === backup.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Download className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setRestoreBackupId(backup.id);
                            setRestoreConfirmed(false);
                          }}
                        >
                          <RotateCcw className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setDeleteBackupId(backup.id)}
                          disabled={isDeleting === backup.id}
                        >
                          {isDeleting === backup.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Trash2 className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Restore Confirmation Dialog */}
      <Dialog open={!!restoreBackupId} onOpenChange={(open) => !open && setRestoreBackupId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-orange-500" />
              Restore from Backup
            </DialogTitle>
            <DialogDescription>
              This will completely replace all current data with the backup contents.
            </DialogDescription>
          </DialogHeader>
          {restoreBackup && (
            <div className="space-y-4">
              <div className="rounded-lg bg-muted p-4 text-sm">
                <p><strong>Backup:</strong> {formatDate(restoreBackup.createdAt)}</p>
                <p><strong>Type:</strong> {restoreBackup.backupType}</p>
                <p><strong>Contents:</strong> {restoreBackup.stats.users} users, {restoreBackup.stats.games} games, {restoreBackup.stats.tags} tags</p>
              </div>
              <div className="rounded-lg border border-orange-200 bg-orange-50 p-4 text-sm text-orange-800 dark:border-orange-800 dark:bg-orange-950 dark:text-orange-200">
                <p className="font-medium">Warning:</p>
                <ul className="mt-1 list-inside list-disc">
                  <li>All current data will be permanently replaced</li>
                  <li>A pre-restore backup will be created automatically</li>
                  <li>You may be logged out after the restore completes</li>
                </ul>
              </div>
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="restore-confirm"
                  checked={restoreConfirmed}
                  onCheckedChange={(checked) => setRestoreConfirmed(checked === true)}
                />
                <Label htmlFor="restore-confirm" className="text-sm">
                  I understand this action cannot be undone
                </Label>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreBackupId(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleRestore}
              disabled={!restoreConfirmed || isRestoring}
            >
              {isRestoring ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Restoring...
                </>
              ) : (
                'Restore'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Upload Restore Dialog */}
      <Dialog open={uploadDialogOpen} onOpenChange={(open) => {
        if (!open) {
          setUploadDialogOpen(false);
          setUploadFile(null);
          setUploadConfirmed(false);
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Upload className="h-5 w-5" />
              Restore from Upload
            </DialogTitle>
            <DialogDescription>
              Upload a backup archive (.tar.gz) to restore from.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Backup File</Label>
              <Input
                type="file"
                accept=".tar.gz,.tgz"
                onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)}
              />
            </div>
            {uploadFile && (
              <>
                <div className="rounded-lg bg-muted p-4 text-sm">
                  <p><strong>File:</strong> {uploadFile.name}</p>
                  <p><strong>Size:</strong> {formatBytes(uploadFile.size)}</p>
                </div>
                <div className="rounded-lg border border-orange-200 bg-orange-50 p-4 text-sm text-orange-800 dark:border-orange-800 dark:bg-orange-950 dark:text-orange-200">
                  <p className="font-medium">Warning:</p>
                  <ul className="mt-1 list-inside list-disc">
                    <li>All current data will be permanently replaced</li>
                    <li>A pre-restore backup will be created automatically</li>
                    <li>The file will be validated before restore</li>
                  </ul>
                </div>
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="upload-confirm"
                    checked={uploadConfirmed}
                    onCheckedChange={(checked) => setUploadConfirmed(checked === true)}
                  />
                  <Label htmlFor="upload-confirm" className="text-sm">
                    I understand this action cannot be undone
                  </Label>
                </div>
              </>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUploadDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleUploadRestore}
              disabled={!uploadFile || !uploadConfirmed || isRestoring}
            >
              {isRestoring ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Restoring...
                </>
              ) : (
                'Restore'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={!!deleteBackupId} onOpenChange={(open) => !open && setDeleteBackupId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Backup?</AlertDialogTitle>
            <AlertDialogDescription>
              This backup will be permanently deleted. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
