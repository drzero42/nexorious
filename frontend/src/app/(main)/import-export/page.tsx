'use client';

import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { toast } from 'sonner';
import {
  useImportNexorious,
  useExportCollection,
} from '@/hooks';
import {
  ImportSource,
  ExportFormat,
  getImportSourceDisplayInfo,
  getExportFormatDisplayInfo,
} from '@/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  AlertCircle,
  Upload,
  Download,
  FileJson,
  FileSpreadsheet,
  Check,
  Loader2,
  ExternalLink,
} from 'lucide-react';

interface ImportCardProps {
  source: ImportSource;
  onFileSelect: (file: File) => void;
  isUploading: boolean;
}

function ImportCard({ source, onFileSelect, isUploading }: ImportCardProps) {
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

  return (
    <Card className={`${colors.bg} ${colors.border} border-2 transition-all ${colors.hover}`}>
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
        />

        <Button
          onClick={handleButtonClick}
          disabled={isUploading}
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
}

function ExportCard({ format, onExport, isExporting }: ExportCardProps) {
  const info = getExportFormatDisplayInfo(format);
  const Icon = format === ExportFormat.JSON ? FileJson : FileSpreadsheet;

  return (
    <Card className="bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 border-2 transition-all hover:border-green-400 dark:hover:border-green-600">
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
          disabled={isExporting}
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
  const router = useRouter();
  const [isUploading, setIsUploading] = useState(false);
  const [exportingCollectionFormat, setExportingCollectionFormat] = useState<ExportFormat | null>(null);

  const { mutateAsync: importNexorious } = useImportNexorious();
  const { mutateAsync: exportCollection } = useExportCollection();

  const handleImportFile = async (file: File) => {
    setIsUploading(true);

    try {
      const result = await importNexorious(file);
      toast.success(`Import started: ${result.message}`);
      router.push(`/jobs/${result.job_id}`);
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
      router.push(`/jobs/${result.job_id}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Export failed';
      toast.error(message);
    } finally {
      setExportingCollectionFormat(null);
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

      {/* Import Section */}
      <section className="mb-8">
        <h2 className="mb-4 text-lg font-semibold">Import Games</h2>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ImportCard
            source={ImportSource.NEXORIOUS}
            onFileSelect={handleImportFile}
            isUploading={isUploading}
          />
        </div>
      </section>

      {/* Export Section */}
      <section className="mb-8">
        <h2 className="mb-4 text-lg font-semibold">Export</h2>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ExportCard
            format={ExportFormat.JSON}
            onExport={() => handleCollectionExport(ExportFormat.JSON)}
            isExporting={exportingCollectionFormat === ExportFormat.JSON}
          />
          <ExportCard
            format={ExportFormat.CSV}
            onExport={() => handleCollectionExport(ExportFormat.CSV)}
            isExporting={exportingCollectionFormat === ExportFormat.CSV}
          />
        </div>
      </section>

      {/* Info Alert */}
      <Alert className="mb-6">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>About Import / Export</AlertTitle>
        <AlertDescription>
          <p className="mb-2">
            <strong>Nexorious JSON</strong> is the recommended format for backups. It preserves all
            metadata including IGDB IDs, ratings, notes, and platform associations.
          </p>
          <p>
            <strong>CSV exports</strong> are useful for spreadsheet analysis but are not
            recommended for re-import due to potential data loss.
          </p>
        </AlertDescription>
      </Alert>

      {/* Quick Links */}
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
