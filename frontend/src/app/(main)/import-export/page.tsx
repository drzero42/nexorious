'use client';

import { useRef, useState } from 'react';
import Link from 'next/link';
import { toast } from 'sonner';
import {
  useImportNexorious,
  useExportCollection,
  useActiveJob,
  useCancelJob,
  useDownloadExport,
} from '@/hooks';
import {
  ImportSource,
  ExportFormat,
  JobType,
  JobStatus,
  getImportSourceDisplayInfo,
  getExportFormatDisplayInfo,
} from '@/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { JobProgressCard, JobItemsDetails, RecentActivity } from '@/components/jobs';
import {
  AlertCircle,
  Upload,
  Download,
  FileJson,
  FileSpreadsheet,
  Check,
  Loader2,
  ExternalLink,
  RotateCcw,
} from 'lucide-react';

interface ImportCardProps {
  source: ImportSource;
  onFileSelect: (file: File) => void;
  isUploading: boolean;
  disabled?: boolean;
}

function ImportCard({ source, onFileSelect, isUploading, disabled }: ImportCardProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const info = getImportSourceDisplayInfo(source);

  const handleButtonClick = () => {
    fileInputRef.current?.click();
  };

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      onFileSelect(file);
      // Reset input so the same file can be selected again
      event.target.value = '';
    }
  };

  const acceptTypes = source === ImportSource.NEXORIOUS
    ? '.json,application/json'
    : '.csv,text/csv';

  const colorClasses = {
    indigo: {
      bg: 'bg-indigo-50 dark:bg-indigo-900/20',
      border: 'border-indigo-200 dark:border-indigo-800',
      hover: 'hover:border-indigo-400 dark:hover:border-indigo-600',
      icon: 'bg-indigo-100 dark:bg-indigo-900/40 text-indigo-600 dark:text-indigo-400',
      button: 'bg-indigo-600 hover:bg-indigo-700',
    },
    purple: {
      bg: 'bg-purple-50 dark:bg-purple-900/20',
      border: 'border-purple-200 dark:border-purple-800',
      hover: 'hover:border-purple-400 dark:hover:border-purple-600',
      icon: 'bg-purple-100 dark:bg-purple-900/40 text-purple-600 dark:text-purple-400',
      button: 'bg-purple-600 hover:bg-purple-700',
    },
  };

  const colors = colorClasses[info.color];
  const isDisabled = disabled || isUploading;

  return (
    <Card className={`${colors.bg} ${colors.border} border-2 transition-all ${!isDisabled ? colors.hover : 'opacity-60'}`}>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-3">
          <div className={`${colors.icon} rounded-lg p-3`}>
            <span className="text-2xl">{info.icon}</span>
          </div>
          <CardTitle className="text-lg">{info.title}</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{info.description}</p>

        <ul className="space-y-2">
          {info.features.map((feature) => (
            <li key={feature} className="flex items-center gap-2 text-sm text-muted-foreground">
              <Check className="h-4 w-4 text-green-500 flex-shrink-0" />
              {feature}
            </li>
          ))}
        </ul>

        <input
          ref={fileInputRef}
          type="file"
          accept={acceptTypes}
          onChange={handleFileChange}
          className="hidden"
          disabled={isDisabled}
        />

        <Button
          onClick={handleButtonClick}
          disabled={isDisabled}
          className={`w-full ${colors.button}`}
        >
          {isUploading ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Uploading...
            </>
          ) : (
            <>
              <Upload className="mr-2 h-4 w-4" />
              Select File
            </>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}

interface ExportCardProps {
  format: ExportFormat;
  onExport: () => void;
  isExporting: boolean;
  disabled?: boolean;
}

function ExportCard({ format, onExport, isExporting, disabled }: ExportCardProps) {
  const info = getExportFormatDisplayInfo(format);
  const Icon = format === ExportFormat.JSON ? FileJson : FileSpreadsheet;
  const isDisabled = disabled || isExporting;

  return (
    <Card className={`bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 border-2 transition-all ${!isDisabled ? 'hover:border-green-400 dark:hover:border-green-600' : 'opacity-60'}`}>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-3">
          <div className="bg-green-100 dark:bg-green-900/40 text-green-600 dark:text-green-400 rounded-lg p-3">
            <Icon className="h-6 w-6" />
          </div>
          <CardTitle className="text-lg">{info.title}</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{info.description}</p>

        <ul className="space-y-2">
          {info.features.map((feature) => (
            <li key={feature} className="flex items-center gap-2 text-sm text-muted-foreground">
              <Check className="h-4 w-4 text-green-500 flex-shrink-0" />
              {feature}
            </li>
          ))}
        </ul>

        <Button
          onClick={onExport}
          disabled={isDisabled}
          className="w-full bg-green-600 hover:bg-green-700"
        >
          {isExporting ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Exporting...
            </>
          ) : (
            <>
              <Download className="mr-2 h-4 w-4" />
              Export {format.toUpperCase()}
            </>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}

export default function ImportExportPage() {
  const [isUploading, setIsUploading] = useState(false);
  const [exportingCollectionFormat, setExportingCollectionFormat] = useState<ExportFormat | null>(null);
  const [dismissedJobId, setDismissedJobId] = useState<string | null>(null);

  const { mutateAsync: importNexorious } = useImportNexorious();
  const { mutateAsync: exportCollection } = useExportCollection();
  const { mutate: cancelJob, isPending: isCancelling } = useCancelJob();
  const { mutate: downloadExport, isPending: isDownloading } = useDownloadExport();

  // Check for active import and export jobs
  const { data: activeImportJob, refetch: refetchImport } = useActiveJob(JobType.IMPORT);
  const { data: activeExportJob, refetch: refetchExport } = useActiveJob(JobType.EXPORT);

  // Determine which job to display
  // Priority: 1) In-progress jobs, 2) Most recently completed job
  const getActiveJob = () => {
    const importNotDismissed = activeImportJob && activeImportJob.id !== dismissedJobId;
    const exportNotDismissed = activeExportJob && activeExportJob.id !== dismissedJobId;

    // First, check for any in-progress job
    if (importNotDismissed && !activeImportJob.isTerminal) return activeImportJob;
    if (exportNotDismissed && !activeExportJob.isTerminal) return activeExportJob;

    // Then, show the most recently completed job
    if (importNotDismissed && exportNotDismissed) {
      // Compare completion times, show the most recent
      const importTime = activeImportJob.completedAt ? new Date(activeImportJob.completedAt).getTime() : 0;
      const exportTime = activeExportJob.completedAt ? new Date(activeExportJob.completedAt).getTime() : 0;
      return exportTime > importTime ? activeExportJob : activeImportJob;
    }

    if (importNotDismissed) return activeImportJob;
    if (exportNotDismissed) return activeExportJob;

    return null;
  };
  const activeJob = getActiveJob();

  // Check if there's an active job that should show inline progress
  const showJobProgress = activeJob != null;
  const hasActiveJob = activeJob != null && !activeJob.isTerminal;
  // Check if the currently displayed job is a completed export (for download button)
  const isActiveJobCompletedExport = activeJob?.isTerminal &&
    activeJob?.status === JobStatus.COMPLETED &&
    activeJob?.jobType === JobType.EXPORT;

  const handleImportFile = async (file: File) => {
    setIsUploading(true);

    try {
      const result = await importNexorious(file);
      toast.success(`Import started: ${result.message}`);
      // Reset dismissed job to show the new job
      setDismissedJobId(null);
      // Refetch to get the new job
      refetchImport();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Import failed';
      toast.error(message);
    } finally {
      setIsUploading(false);
    }
  };

  const handleCollectionExport = async (format: ExportFormat) => {
    setExportingCollectionFormat(format);

    try {
      const result = await exportCollection(format);
      toast.success(`Export started: ${result.message}`);
      // Reset dismissed job to show the new job
      setDismissedJobId(null);
      // Refetch to get the new job
      refetchExport();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Export failed';
      toast.error(message);
    } finally {
      setExportingCollectionFormat(null);
    }
  };

  const handleCancelJob = async () => {
    if (!activeJob) return;

    cancelJob(activeJob.id, {
      onSuccess: () => {
        toast.success('Job cancelled');
        refetchImport();
        refetchExport();
      },
      onError: (error) => {
        toast.error(error.message || 'Failed to cancel job');
      },
    });
  };

  const handleDownloadExport = () => {
    if (!activeJob || activeJob.jobType !== JobType.EXPORT) return;

    downloadExport(activeJob.id, {
      onSuccess: () => {
        toast.success('Download started');
      },
      onError: (error) => {
        toast.error(error.message || 'Failed to download export');
      },
    });
  };

  const handleDismissJob = () => {
    if (activeJob) {
      setDismissedJobId(activeJob.id);
    }
  };

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Import / Export</span>
        </nav>
        <h1 className="text-2xl font-bold">Import / Export</h1>
        <p className="text-muted-foreground">
          Import your game collection from various sources or export your data for backup.
        </p>
      </div>

      {/* Active Job Progress View */}
      {showJobProgress && activeJob && (
        <section className="mb-8 space-y-4">
          <JobProgressCard
            job={activeJob}
            onCancel={handleCancelJob}
            isCancelling={isCancelling}
          />

          {activeJob.progress && (
            <JobItemsDetails
              jobId={activeJob.id}
              progress={activeJob.progress}
              isTerminal={activeJob.isTerminal}
            />
          )}

          {/* Actions for completed jobs */}
          {activeJob.isTerminal && (
            <div className="flex gap-3">
              {/* Download button for completed exports */}
              {isActiveJobCompletedExport && (
                <Button
                  onClick={handleDownloadExport}
                  disabled={isDownloading}
                  className="bg-green-600 hover:bg-green-700"
                >
                  {isDownloading ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Downloading...
                    </>
                  ) : (
                    <>
                      <Download className="mr-2 h-4 w-4" />
                      Download Export
                    </>
                  )}
                </Button>
              )}

              {/* Start New button */}
              <Button
                variant="outline"
                onClick={handleDismissJob}
              >
                <RotateCcw className="mr-2 h-4 w-4" />
                Start New
              </Button>
            </div>
          )}
        </section>
      )}

      {/* Import Section - hidden only when job is in progress */}
      {!hasActiveJob && (
        <section className="mb-8">
          <h2 className="mb-4 text-lg font-semibold">Import Games</h2>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <ImportCard
              source={ImportSource.NEXORIOUS}
              onFileSelect={handleImportFile}
              isUploading={isUploading}
              disabled={hasActiveJob}
            />
          </div>
        </section>
      )}

      {/* Export Section - hidden only when job is in progress */}
      {!hasActiveJob && (
        <section className="mb-8">
          <h2 className="mb-4 text-lg font-semibold">Export</h2>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <ExportCard
              format={ExportFormat.JSON}
              onExport={() => handleCollectionExport(ExportFormat.JSON)}
              isExporting={exportingCollectionFormat === ExportFormat.JSON}
              disabled={hasActiveJob}
            />
            <ExportCard
              format={ExportFormat.CSV}
              onExport={() => handleCollectionExport(ExportFormat.CSV)}
              isExporting={exportingCollectionFormat === ExportFormat.CSV}
              disabled={hasActiveJob}
            />
          </div>
        </section>
      )}

      {/* Info Alert - always visible */}
      <Alert className="mb-6">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>About Import / Export</AlertTitle>
        <AlertDescription>
          <p className="mb-2">
            <strong>Nexorious JSON</strong> is the recommended format for importing on other Nexorious instances. It preserves all
            metadata including IGDB IDs, ratings, notes, and platform associations.
          </p>
          <p>
            <strong>CSV exports</strong> are useful for spreadsheet analysis but are not
            recommended for re-import due to potential data loss.
          </p>
        </AlertDescription>
      </Alert>

      {/* Recent Activity - shows completed jobs from last 7 days */}
      {!showJobProgress && (
        <section className="mb-6">
          <RecentActivity
            excludeJobIds={[activeImportJob?.id, activeExportJob?.id].filter((id): id is string => !!id)}
          />
        </section>
      )}

      {/* Quick Links - always visible */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Links</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Link
            href="/sync"
            className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
          >
            <div>
              <div className="font-medium">Sync Settings</div>
              <div className="text-sm text-muted-foreground">
                Connect and sync your Steam library
              </div>
            </div>
            <ExternalLink className="h-4 w-4 text-muted-foreground" />
          </Link>
          <Link
            href="/games"
            className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
          >
            <div>
              <div className="font-medium">View Collection</div>
              <div className="text-sm text-muted-foreground">
                Browse and manage your game library
              </div>
            </div>
            <ExternalLink className="h-4 w-4 text-muted-foreground" />
          </Link>
        </CardContent>
      </Card>
    </div>
  );
}
