# Export to File Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the export-to-file feature by adding missing frontend functionality and wishlist exports.

**Architecture:** The backend is fully implemented. We need to add: (1) wishlist export buttons to the UI, (2) a download button on the job detail page for completed exports, and (3) frontend tests.

**Tech Stack:** Next.js 16, React 19, TanStack Query, shadcn/ui, Vitest

---

## Summary of What's Already Built

**Backend (complete):**
- `/api/export/collection/json` and `/api/export/collection/csv` endpoints
- `/api/export/wishlist/json` and `/api/export/wishlist/csv` endpoints
- `/api/export/{job_id}/download` endpoint with expiration handling
- Export worker task that generates JSON/CSV files
- Comprehensive test coverage

**Frontend (partial):**
- API functions: `exportCollectionJson()`, `exportCollectionCsv()`, `downloadExport()`
- Hooks: `useExportCollection()`, `useDownloadExport()`
- Types: `ExportFormat`, `ExportScope`, `ExportJobCreatedResponse`
- Import/Export page with collection export cards

**Missing:**
1. Wishlist export functions and UI
2. Download button on job detail page for completed exports
3. Frontend tests for new functionality

---

### Task 1: Add Wishlist Export API Functions

**Files:**
- Modify: `frontend/src/api/import-export.ts`

**Step 1: Add wishlist export functions to API**

Add these functions after `exportCollectionCsv()`:

```typescript
/**
 * Start a JSON export of the user's wishlist.
 */
export async function exportWishlistJson(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobApiResponse>('/export/wishlist/json');
  return transformExportJobResponse(response);
}

/**
 * Start a CSV export of the user's wishlist.
 */
export async function exportWishlistCsv(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobApiResponse>('/export/wishlist/csv');
  return transformExportJobResponse(response);
}
```

**Step 2: Commit**

```bash
git add frontend/src/api/import-export.ts
git commit -m "feat(api): add wishlist export API functions"
```

---

### Task 2: Add Wishlist Export Hook

**Files:**
- Modify: `frontend/src/hooks/use-import-export.ts`

**Step 1: Add wishlist export hook**

Add this hook after `useExportCollection()`:

```typescript
/**
 * Hook to start an export of the user's wishlist.
 * Returns the job ID for tracking progress.
 */
export function useExportWishlist() {
  return useMutation<ExportJobCreatedResponse, Error, ExportFormat>({
    mutationFn: (format) => {
      if (format === 'json') {
        return importExportApi.exportWishlistJson();
      }
      return importExportApi.exportWishlistCsv();
    },
  });
}
```

**Step 2: Commit**

```bash
git add frontend/src/hooks/use-import-export.ts
git commit -m "feat(hooks): add useExportWishlist hook"
```

---

### Task 3: Export Hooks from Index

**Files:**
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Verify exports**

Check if `useExportWishlist` needs to be added to the exports. If hooks are auto-exported via barrel file, verify the export exists.

**Step 2: Add export if needed**

If not auto-exported, add:

```typescript
export { useExportWishlist } from './use-import-export';
```

**Step 3: Commit if changes made**

```bash
git add frontend/src/hooks/index.ts
git commit -m "feat(hooks): export useExportWishlist from index"
```

---

### Task 4: Add Wishlist Export Section to UI

**Files:**
- Modify: `frontend/src/app/(main)/import-export/page.tsx`

**Step 1: Import the new hook**

Update the imports:

```typescript
import {
  useImportNexorious,
  useImportDarkadia,
  useExportCollection,
  useExportWishlist,
} from '@/hooks';
```

**Step 2: Add state and mutation for wishlist exports**

In the `ImportExportPage` component, add after existing state:

```typescript
const [exportingWishlistFormat, setExportingWishlistFormat] = useState<ExportFormat | null>(null);
const { mutateAsync: exportWishlist } = useExportWishlist();
```

**Step 3: Add handler for wishlist export**

```typescript
const handleWishlistExport = async (format: ExportFormat) => {
  setExportingWishlistFormat(format);

  try {
    const result = await exportWishlist(format);
    toast.success(`Wishlist export started: ${result.message}`);
    router.push(`/jobs/${result.job_id}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Export failed';
    toast.error(message);
  } finally {
    setExportingWishlistFormat(null);
  }
};
```

**Step 4: Add Wishlist Export section to JSX**

Add after the existing Export Section:

```tsx
{/* Wishlist Export Section */}
<section className="mb-8">
  <h2 className="mb-4 text-lg font-semibold">Export Wishlist</h2>
  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
    <ExportCard
      format={ExportFormat.JSON}
      onExport={() => handleWishlistExport(ExportFormat.JSON)}
      isExporting={exportingWishlistFormat === ExportFormat.JSON}
      scope="wishlist"
    />
    <ExportCard
      format={ExportFormat.CSV}
      onExport={() => handleWishlistExport(ExportFormat.CSV)}
      isExporting={exportingWishlistFormat === ExportFormat.CSV}
      scope="wishlist"
    />
  </div>
</section>
```

**Step 5: Update ExportCard to accept optional scope prop**

Update the `ExportCardProps` interface and component:

```typescript
interface ExportCardProps {
  format: ExportFormat;
  onExport: () => void;
  isExporting: boolean;
  scope?: 'collection' | 'wishlist';
}

function ExportCard({ format, onExport, isExporting, scope = 'collection' }: ExportCardProps) {
  const info = getExportFormatDisplayInfo(format);
  const Icon = format === ExportFormat.JSON ? FileJson : FileSpreadsheet;
  const scopeLabel = scope === 'wishlist' ? 'Wishlist' : 'Collection';

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
              Export {scopeLabel} {format.toUpperCase()}
            </>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}
```

**Step 6: Commit**

```bash
git add frontend/src/app/\(main\)/import-export/page.tsx
git commit -m "feat(ui): add wishlist export section to import/export page"
```

---

### Task 5: Add Download Button to Job Detail Page

**Files:**
- Modify: `frontend/src/app/(main)/jobs/[id]/page.tsx`

**Step 1: Import download hook and icon**

Add to imports:

```typescript
import { useJob, useCancelJob, useDeleteJob, useConfirmJob, useDownloadExport } from '@/hooks';
import { Download } from 'lucide-react'; // Add Download to existing lucide imports
```

**Step 2: Add download mutation and helper**

In the component, add after other mutations:

```typescript
const downloadExportMutation = useDownloadExport();

// Helper to check if job is a completed export
const isCompletedExport = job?.jobType === 'export' && job?.status === JobStatus.COMPLETED;
```

**Step 3: Add download handler**

```typescript
const handleDownload = async () => {
  if (!job) return;
  try {
    await downloadExportMutation.mutateAsync(job.id);
    toast.success('Download started');
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to download export');
  }
};
```

**Step 4: Add Download button to CardFooter**

In the CardFooter, add the download button before other action buttons:

```tsx
{isCompletedExport && (
  <Button
    onClick={handleDownload}
    disabled={downloadExportMutation.isPending}
    className="bg-green-600 hover:bg-green-700"
  >
    {downloadExportMutation.isPending ? (
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
    ) : (
      <Download className="mr-2 h-4 w-4" />
    )}
    Download Export
  </Button>
)}
```

**Step 5: Commit**

```bash
git add frontend/src/app/\(main\)/jobs/\[id\]/page.tsx
git commit -m "feat(ui): add download button for completed export jobs"
```

---

### Task 6: Add Tests for Wishlist Export API Functions

**Files:**
- Modify: `frontend/src/api/import-export.test.ts`

**Step 1: Read existing test file to understand patterns**

Run: `cat frontend/src/api/import-export.test.ts`

**Step 2: Add tests for wishlist export functions**

```typescript
describe('exportWishlistJson', () => {
  it('should call POST /export/wishlist/json and return transformed response', async () => {
    const mockResponse: ExportJobApiResponse = {
      job_id: 'wishlist-job-123',
      status: 'pending',
      message: 'Wishlist export started',
      estimated_items: 10,
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await exportWishlistJson();

    expect(mockApi.post).toHaveBeenCalledWith('/export/wishlist/json');
    expect(result).toEqual({
      job_id: 'wishlist-job-123',
      status: 'pending',
      message: 'Wishlist export started',
      estimated_items: 10,
    });
  });
});

describe('exportWishlistCsv', () => {
  it('should call POST /export/wishlist/csv and return transformed response', async () => {
    const mockResponse: ExportJobApiResponse = {
      job_id: 'wishlist-csv-job-123',
      status: 'pending',
      message: 'Wishlist CSV export started',
      estimated_items: 5,
    };

    mockApi.post.mockResolvedValueOnce(mockResponse);

    const result = await exportWishlistCsv();

    expect(mockApi.post).toHaveBeenCalledWith('/export/wishlist/csv');
    expect(result).toEqual({
      job_id: 'wishlist-csv-job-123',
      status: 'pending',
      message: 'Wishlist CSV export started',
      estimated_items: 5,
    });
  });
});
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- import-export.test.ts`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/api/import-export.test.ts
git commit -m "test(api): add tests for wishlist export functions"
```

---

### Task 7: Add Tests for Wishlist Export Hook

**Files:**
- Modify: `frontend/src/hooks/use-import-export.test.ts`

**Step 1: Read existing test file**

Run: `cat frontend/src/hooks/use-import-export.test.ts`

**Step 2: Add tests for useExportWishlist hook**

```typescript
describe('useExportWishlist', () => {
  it('should call exportWishlistJson when format is json', async () => {
    const mockResponse: ExportJobCreatedResponse = {
      job_id: 'wishlist-json-job',
      status: 'pending',
      message: 'Export started',
      estimated_items: 10,
    };

    vi.mocked(importExportApi.exportWishlistJson).mockResolvedValueOnce(mockResponse);

    const { result } = renderHook(() => useExportWishlist(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync('json' as ExportFormat);
    });

    expect(importExportApi.exportWishlistJson).toHaveBeenCalled();
    expect(importExportApi.exportWishlistCsv).not.toHaveBeenCalled();
  });

  it('should call exportWishlistCsv when format is csv', async () => {
    const mockResponse: ExportJobCreatedResponse = {
      job_id: 'wishlist-csv-job',
      status: 'pending',
      message: 'Export started',
      estimated_items: 5,
    };

    vi.mocked(importExportApi.exportWishlistCsv).mockResolvedValueOnce(mockResponse);

    const { result } = renderHook(() => useExportWishlist(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync('csv' as ExportFormat);
    });

    expect(importExportApi.exportWishlistCsv).toHaveBeenCalled();
    expect(importExportApi.exportWishlistJson).not.toHaveBeenCalled();
  });
});
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- use-import-export.test.ts`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/hooks/use-import-export.test.ts
git commit -m "test(hooks): add tests for useExportWishlist hook"
```

---

### Task 8: Run Full Test Suite and Type Check

**Step 1: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No TypeScript errors

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 3: Fix any issues found**

If tests or type checks fail, fix the issues before proceeding.

**Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: resolve test and type check issues"
```

---

### Task 9: Manual Testing Verification

**Step 1: Start the development servers**

```bash
# Terminal 1: Backend
cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main

# Terminal 2: Frontend
cd /home/abo/workspace/home/nexorious/frontend && npm run dev
```

**Step 2: Test export flows**

1. Navigate to `/import-export`
2. Verify both Collection and Wishlist export sections appear
3. Click "Export Collection JSON" - verify redirect to job page
4. Wait for job to complete - verify "Download Export" button appears
5. Click download - verify file downloads correctly
6. Repeat for CSV exports and wishlist exports

**Step 3: Document any issues found for fixing**

---

## Verification Checklist

- [ ] Wishlist JSON export works end-to-end
- [ ] Wishlist CSV export works end-to-end
- [ ] Download button appears for completed export jobs
- [ ] Download triggers file download in browser
- [ ] All frontend tests pass
- [ ] Type checking passes
- [ ] No console errors in browser
