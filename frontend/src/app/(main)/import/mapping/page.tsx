'use client';

import { useEffect, useMemo } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import { AlertCircle, ArrowRight } from 'lucide-react';
import { MappingSection } from '@/components/import/mapping-section';
import { useImportMapping } from '@/contexts/import-mapping-context';
import { usePlatformSummary, useAllPlatforms, useAllStorefronts } from '@/hooks';

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

  const isLoading = summaryLoading || platformsLoading || storefrontsLoading;

  // Set job ID in context when page loads
  useEffect(() => {
    if (jobId) {
      setJobId(jobId);
    }
  }, [jobId, setJobId]);

  // Redirect to review if all resolved
  useEffect(() => {
    if (summary?.allResolved && jobId) {
      router.replace(`/review?job_id=${jobId}`);
    }
  }, [summary?.allResolved, jobId, router]);

  // Get unresolved items
  const unresolvedPlatforms = useMemo(
    () => summary?.platforms.filter((p) => !p.suggestedId) || [],
    [summary?.platforms]
  );
  const unresolvedStorefronts = useMemo(
    () => summary?.storefronts.filter((s) => !s.suggestedId) || [],
    [summary?.storefronts]
  );

  // Check if all unresolved items have mappings
  const allMapped = useMemo(() => {
    const platformsMapped = unresolvedPlatforms.every(
      (p) => platformMappings[p.original]
    );
    const storefrontsMapped = unresolvedStorefronts.every(
      (s) => storefrontMappings[s.original]
    );
    return platformsMapped && storefrontsMapped;
  }, [unresolvedPlatforms, unresolvedStorefronts, platformMappings, storefrontMappings]);

  const handleContinue = () => {
    if (jobId) {
      router.push(`/review?job_id=${jobId}`);
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
          Some values from your CSV need to be mapped to our system. Please select the correct
          mapping for each unrecognized value below.
        </p>
      </div>

      {/* Mapping Sections */}
      <Card>
        <CardHeader>
          <CardTitle>Unresolved Mappings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-8">
          {summary && platforms && (
            <MappingSection
              title="Platforms"
              items={summary.platforms}
              options={platforms.map((p) => ({ id: p.id, display_name: p.display_name }))}
              mappings={platformMappings}
              onMappingChange={setPlatformMapping}
            />
          )}

          {summary && storefronts && (
            <MappingSection
              title="Storefronts"
              items={summary.storefronts}
              options={storefronts.map((s) => ({ id: s.id, display_name: s.display_name }))}
              mappings={storefrontMappings}
              onMappingChange={setStorefrontMapping}
            />
          )}

          {unresolvedPlatforms.length === 0 && unresolvedStorefronts.length === 0 && (
            <p className="text-center text-muted-foreground">
              All platforms and storefronts have been automatically matched.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Continue Button */}
      <div className="flex justify-end">
        <Button onClick={handleContinue} disabled={!allMapped} size="lg">
          Continue to Review
          <ArrowRight className="ml-2 h-4 w-4" />
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
