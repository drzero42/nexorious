import { useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Loader2, Upload } from 'lucide-react';
import { PlayStatus } from '@/types';
import type { CsvInspectResponse, CsvMapping, CsvPresetInfo } from '@/types';
import { statusLabels } from '@/lib/play-status';
import { emptyCsvMapping, initStatusValueMap, availableHeaders } from './csv-mapping';

// shadcn Select cannot hold an empty-string value, so "no column" uses a sentinel.
const NONE = '__none__';

const OPTIONAL_FIELDS = [
  { key: 'igdb_id', label: 'IGDB ID' },
  { key: 'platform', label: 'Platform' },
  { key: 'storefront', label: 'Storefront' },
  { key: 'rating', label: 'Rating' },
  { key: 'notes', label: 'Notes' },
  { key: 'acquired_date', label: 'Acquired date' },
  { key: 'hours_played', label: 'Hours played' },
  { key: 'tags', label: 'Tags' },
  { key: 'loved', label: 'Loved' },
] as const;

type OptionalKey = (typeof OPTIONAL_FIELDS)[number]['key'];

interface CsvMappingDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  inspect: CsvInspectResponse;
  isImporting: boolean;
  onImport: (result: { format: string; mapping: CsvMapping }) => void;
}

// CsvMappingDialog is the Dialog shell. The form body is mounted only while open
// so its local mapping state resets on every reopen (matching the inner-body
// pattern other dialogs in this app use) — otherwise a second CSV upload would
// inherit the previous file's column selections.
export function CsvMappingDialog({
  open,
  onOpenChange,
  inspect,
  isImporting,
  onImport,
}: CsvMappingDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        {open && (
          <CsvMappingForm
            inspect={inspect}
            isImporting={isImporting}
            onImport={onImport}
            onCancel={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

interface CsvMappingFormProps {
  inspect: CsvInspectResponse;
  isImporting: boolean;
  onImport: (result: { format: string; mapping: CsvMapping }) => void;
  onCancel: () => void;
}

function CsvMappingForm({ inspect, isImporting, onImport, onCancel }: CsvMappingFormProps) {
  const [mapping, setMapping] = useState<CsvMapping>(
    () => inspect.suggested_mapping ?? emptyCsvMapping(),
  );
  const [format, setFormat] = useState('generic');
  const isPreset = format !== 'generic';
  const presets: CsvPresetInfo[] = inspect.presets ?? [];

  const setColumn = (key: 'title' | OptionalKey, raw: string) => {
    const value = raw === NONE ? '' : raw;
    setMapping((m) => ({ ...m, columns: { ...m.columns, [key]: value } }));
  };

  const handleStatusColumn = (raw: string) => {
    const column = raw === NONE ? '' : raw;
    const distinct = column
      ? (inspect.columns.find((c) => c.name === column)?.distinct_values ?? [])
      : [];
    setMapping((m) => ({ ...m, status: { column, value_map: initStatusValueMap(distinct) } }));
  };

  const statusColumnInfo = useMemo(
    () => inspect.columns.find((c) => c.name === mapping.status.column),
    [inspect.columns, mapping.status.column],
  );

  const columnSelect = (
    id: string,
    label: string,
    value: string,
    onChange: (raw: string) => void,
  ) => (
    <Select value={value || NONE} onValueChange={onChange}>
      <SelectTrigger id={id} aria-label={`${label} column`}>
        <SelectValue placeholder="— none —" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NONE}>— none —</SelectItem>
        {availableHeaders(inspect.headers, mapping, value).map((h) => (
          <SelectItem key={h} value={h}>
            {h}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );

  return (
    <>
      <DialogHeader>
        <DialogTitle>Import CSV</DialogTitle>
        <DialogDescription>
          {inspect.row_count} rows detected. Map your CSV columns to Nexorious fields.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-5">
        <div className="grid grid-cols-2 items-center gap-2">
          <Label htmlFor="csv-format">Format</Label>
          <Select value={format} onValueChange={setFormat}>
            <SelectTrigger id="csv-format" aria-label="Format">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="generic">Generic CSV</SelectItem>
              {presets.map((p) => (
                <SelectItem key={p.slug} value={p.slug}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {isPreset && (
          <p className="text-sm text-muted-foreground">
            Columns, play-status, platforms, ratings and dates are mapped automatically by the{' '}
            {presets.find((p) => p.slug === format)?.name ?? format} preset.
          </p>
        )}

        {!isPreset && (
          <section className="space-y-3">
            <h4 className="text-sm font-semibold">1 · Map columns</h4>

            <div className="grid grid-cols-2 items-center gap-2">
              <Label htmlFor="csv-title">
                Title <span className="text-red-500">*</span>
              </Label>
              {columnSelect('csv-title', 'Title', mapping.columns.title, (raw) =>
                setColumn('title', raw),
              )}
            </div>

            <div className="grid grid-cols-2 items-center gap-2">
              <Label htmlFor="csv-status">Play status</Label>
              {columnSelect('csv-status', 'Play status', mapping.status.column, handleStatusColumn)}
            </div>

            {OPTIONAL_FIELDS.map((f) => (
              <div key={f.key} className="grid grid-cols-2 items-center gap-2">
                <Label htmlFor={`csv-${f.key}`}>{f.label}</Label>
                <div className="flex items-center gap-2">
                  {columnSelect(`csv-${f.key}`, f.label, mapping.columns[f.key], (raw) =>
                    setColumn(f.key, raw),
                  )}
                  {f.key === 'rating' && mapping.columns.rating && (
                    <Select
                      value={String(mapping.rating_scale)}
                      onValueChange={(v) => setMapping((m) => ({ ...m, rating_scale: Number(v) }))}
                    >
                      <SelectTrigger aria-label="Rating scale" className="w-28">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="5">out of 5</SelectItem>
                        <SelectItem value="10">out of 10</SelectItem>
                        <SelectItem value="100">out of 100</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                </div>
              </div>
            ))}

            <div className="flex items-center gap-2 pt-1">
              <Switch
                id="csv-merge"
                checked={mapping.merge_by_title}
                onCheckedChange={(checked) =>
                  setMapping((m) => ({ ...m, merge_by_title: checked }))
                }
              />
              <Label htmlFor="csv-merge">Merge rows with the same title</Label>
            </div>
          </section>
        )}

        {!isPreset &&
          mapping.status.column &&
          (statusColumnInfo?.distinct_values.length ?? 0) > 0 && (
            <section className="space-y-3">
              <h4 className="text-sm font-semibold">2 · Map status values</h4>
              {statusColumnInfo?.distinct_truncated && (
                <p className="text-xs text-muted-foreground">
                  Showing the first {statusColumnInfo.distinct_values.length} values; any others
                  import as Not Started.
                </p>
              )}
              {statusColumnInfo?.distinct_values.map((value) => (
                <div key={value} className="grid grid-cols-2 items-center gap-2">
                  <Label htmlFor={`csv-sv-${value}`} className="truncate">
                    "{value}"
                  </Label>
                  <Select
                    value={mapping.status.value_map[value] ?? PlayStatus.NOT_STARTED}
                    onValueChange={(v) =>
                      setMapping((m) => ({
                        ...m,
                        status: { ...m.status, value_map: { ...m.status.value_map, [value]: v } },
                      }))
                    }
                  >
                    <SelectTrigger id={`csv-sv-${value}`} aria-label={`Status for ${value}`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {Object.values(PlayStatus).map((ps) => (
                        <SelectItem key={ps} value={ps}>
                          {statusLabels[ps]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ))}
            </section>
          )}
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={onCancel} disabled={isImporting}>
          Cancel
        </Button>
        <Button
          onClick={() => onImport({ format, mapping })}
          disabled={(!isPreset && !mapping.columns.title) || isImporting}
        >
          {isImporting ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Importing...
            </>
          ) : (
            <>
              <Upload className="mr-2 h-4 w-4" />
              Import {inspect.row_count} games
            </>
          )}
        </Button>
      </DialogFooter>
    </>
  );
}
