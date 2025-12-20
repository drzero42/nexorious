'use client';

import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

interface ImportMappingContextValue {
  jobId: string | null;
  platformMappings: Record<string, string>;
  storefrontMappings: Record<string, string>;
  setJobId: (jobId: string | null) => void;
  setPlatformMapping: (original: string, resolvedId: string) => void;
  setStorefrontMapping: (original: string, resolvedId: string) => void;
  clearMappings: () => void;
}

const ImportMappingContext = createContext<ImportMappingContextValue | null>(null);

export function ImportMappingProvider({ children }: { children: ReactNode }) {
  const [jobId, setJobId] = useState<string | null>(null);
  const [platformMappings, setPlatformMappings] = useState<Record<string, string>>({});
  const [storefrontMappings, setStorefrontMappings] = useState<Record<string, string>>({});

  const setPlatformMapping = useCallback((original: string, resolvedId: string) => {
    setPlatformMappings((prev) => ({ ...prev, [original]: resolvedId }));
  }, []);

  const setStorefrontMapping = useCallback((original: string, resolvedId: string) => {
    setStorefrontMappings((prev) => ({ ...prev, [original]: resolvedId }));
  }, []);

  const clearMappings = useCallback(() => {
    setJobId(null);
    setPlatformMappings({});
    setStorefrontMappings({});
  }, []);

  return (
    <ImportMappingContext.Provider
      value={{
        jobId,
        platformMappings,
        storefrontMappings,
        setJobId,
        setPlatformMapping,
        setStorefrontMapping,
        clearMappings,
      }}
    >
      {children}
    </ImportMappingContext.Provider>
  );
}

export function useImportMapping(): ImportMappingContextValue {
  const context = useContext(ImportMappingContext);
  if (!context) {
    throw new Error('useImportMapping must be used within an ImportMappingProvider');
  }
  return context;
}
