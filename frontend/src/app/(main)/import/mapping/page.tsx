'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import { AlertCircle, ArrowRight, Loader2 } from 'lucide-react';
import { MappingSection } from '@/components/import/mapping-section';
import { useImportMapping } from '@/contexts/import-mapping-context';
import {
  usePlatformSummary,
  useAllPlatforms,
  useAllStorefronts,
  useJob,
  useBatchImportMappings,
} from '@/hooks';
import { MappingType } from '@/types';

export default function MappingPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const jobId = searchParams.get('job_id');

  const {
    platformMappings,
    storefrontMappings,
    setJobId,
    setPlatformMapping,
    setStorefrontMapping,
  } = useImportMapping();

  const { data: summary, isLoading: summaryLoading, error: summaryError } = usePlatformSummary(jobId);
  const { data: platforms, isLoading: platformsLoading } = useAllPlatforms({ activeOnly: true });
  const { data: storefronts, isLoading: storefrontsLoading } = useAllStorefronts({ activeOnly: true });
  const { data: job, isLoading: jobLoading } = useJob(jobId || undefined);
  const batchImportMappings = useBatchImportMappings();
  const [isSaving, setIsSaving] = useState(false);

  const isLoading = summaryLoading || platformsLoading || storefrontsLoading || jobLoading;

  // Set job ID in context when page loads
  useEffect(() => {
    if (jobId) {
      setJobId(jobId);
    }
  }, [jobId, setJobId]);

  // Pre-populate mappings from auto-resolved suggestions
  useEffect(() => {
    if (summary) {
      // Pre-populate platform mappings from suggestions
      summary.platforms.forEach((p) => {
        if (p.suggestedId && !platformMappings[p.original]) {
          setPlatformMapping(p.original, p.suggestedId);
        }
      });
      // Pre-populate storefront mappings from suggestions
      summary.storefronts.forEach((s) => {
        if (s.suggestedId && !storefrontMappings[s.original]) {
          setStorefrontMapping(s.original, s.suggestedId);
        }
      });
    }
  }, [summary, platformMappings, storefrontMappings, setPlatformMapping, setStorefrontMapping]);

  // Check if all items have mappings (either from suggestions or user selection)
  const allMapped = useMemo(() => {
    if (!summary) return false;
    const platformsMapped = summary.platforms.every(
      (p) => platformMappings[p.original] || p.suggestedId
    );
    const storefrontsMapped = summary.storefronts.every(
      (s) => storefrontMappings[s.original] || s.suggestedId
    );
    return platformsMapped && storefrontsMapped;
  }, [summary, platformMappings, storefrontMappings]);

  // Count unresolved items (no suggestion and no manual mapping)
  const unresolvedCount = useMemo(() => {
    if (!summary) return 0;
    const unresolvedPlatforms = summary.platforms.filter(
      (p) => !p.suggestedId && !platformMappings[p.original]
    ).length;
    const unresolvedStorefronts = summary.storefronts.filter(
      (s) => !s.suggestedId && !storefrontMappings[s.original]
    ).length;
    return unresolvedPlatforms + unresolvedStorefronts;
  }, [summary, platformMappings, storefrontMappings]);

  const handleContinue = async () => {
    if (!jobId || !job || !summary) return;

    setIsSaving(true);
    try {
      // Build the list of mappings to save
      const mappingsToSave = [
        // Platform mappings
        ...summary.platforms.map((p) => ({
          mappingType: MappingType.PLATFORM,
          sourceValue: p.original,
          targetId: platformMappings[p.original] || p.suggestedId || '',
        })),
        // Storefront mappings
        ...summary.storefronts.map((s) => ({
          mappingType: MappingType.STOREFRONT,
          sourceValue: s.original,
          targetId: storefrontMappings[s.original] || s.suggestedId || '',
        })),
      ].filter((m) => m.targetId); // Only save mappings with a target

      if (mappingsToSave.length > 0) {
        // Save mappings to backend using the job's source as the import source
        await batchImportMappings.mutateAsync({
          importSource: job.source,
          mappings: mappingsToSave,
        });
      }

      // Navigate to review page
      router.push(`/review?job_id=${jobId}`);
    } catch (error) {
      console.error('Failed to save mappings:', error);
      // Still navigate even if save fails - mappings are also in context
      router.push(`/review?job_id=${jobId}`);
    } finally {
      setIsSaving(false);
    }
  };

  if (!jobId) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>No job ID provided. Please start an import first.</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div>
          <Skeleton className="mb-2 h-8 w-64" />
          <Skeleton className="h-4 w-96" />
        </div>
        <Card>
          <CardContent className="space-y-4 p-6">
            <Skeleton className="h-6 w-32" />
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      </div>
    );
  }

  if (summaryError) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {summaryError instanceof Error ? summaryError.message : 'Failed to load platform summary'}
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link href="/import-export" className="hover:text-foreground">
            Import / Export
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Platform Mapping</span>
        </nav>
        <h1 className="text-2xl font-bold">Platform & Storefront Mapping</h1>
        <p className="text-muted-foreground">
          Review and confirm the platform/storefront mappings from your CSV import.
          {unresolvedCount > 0
            ? ` ${unresolvedCount} item${unresolvedCount > 1 ? 's' : ''} need${unresolvedCount === 1 ? 's' : ''} manual mapping.`
            : ' All items have been automatically matched.'}
        </p>
      </div>

      {/* Mapping Sections */}
      <Card>
        <CardHeader>
          <CardTitle>Platform & Storefront Mappings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-8">
          {summary && platforms && summary.platforms.length > 0 && (
            <MappingSection
              title="Platforms"
              items={summary.platforms}
              options={platforms.map((p) => ({ id: p.id, display_name: p.display_name }))}
              mappings={platformMappings}
              onMappingChange={setPlatformMapping}
            />
          )}

          {summary && storefronts && summary.storefronts.length > 0 && (
            <MappingSection
              title="Storefronts"
              items={summary.storefronts}
              options={storefronts.map((s) => ({ id: s.id, display_name: s.display_name }))}
              mappings={storefrontMappings}
              onMappingChange={setStorefrontMapping}
            />
          )}

          {summary && summary.platforms.length === 0 && summary.storefronts.length === 0 && (
            <p className="text-center text-muted-foreground">
              No platform or storefront data found in the import.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Continue Button */}
      <div className="flex justify-end">
        <Button onClick={handleContinue} disabled={!allMapped || isSaving} size="lg">
          {isSaving ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Saving Mappings...
            </>
          ) : (
            <>
              Continue to Review
              <ArrowRight className="ml-2 h-4 w-4" />
            </>
          )}
        </Button>
      </div>

      {!allMapped && (
        <p className="text-right text-sm text-muted-foreground">
          Please map all unresolved values before continuing.
        </p>
      )}
    </div>
  );
}
