/**
 * Types for import/export functionality.
 */

export enum ImportSource {
  NEXORIOUS = 'nexorious',
  DARKADIA = 'darkadia',
}

export enum ExportFormat {
  JSON = 'json',
  CSV = 'csv',
}

export enum ExportScope {
  COLLECTION = 'collection',
  WISHLIST = 'wishlist',
}

export interface ImportJobCreatedResponse {
  job_id: string;
  source: string;
  status: string;
  message: string;
  total_items: number | null;
}

export interface ExportJobCreatedResponse {
  job_id: string;
  status: string;
  message: string;
  estimated_items: number;
}

// Helper to get import source display info
export function getImportSourceDisplayInfo(source: ImportSource): {
  title: string;
  description: string;
  icon: string;
  features: string[];
  color: 'indigo' | 'purple';
} {
  const info: Record<ImportSource, {
    title: string;
    description: string;
    icon: string;
    features: string[];
    color: 'indigo' | 'purple';
  }> = {
    [ImportSource.NEXORIOUS]: {
      title: 'Nexorious JSON',
      description: 'Restore a previous Nexorious export with all metadata, ratings, play status, and notes intact.',
      icon: '📦',
      features: ['Full metadata restoration', 'Preserves ratings and notes', 'Non-interactive import'],
      color: 'indigo',
    },
    [ImportSource.DARKADIA]: {
      title: 'Darkadia CSV',
      description: 'Import your game collection from Darkadia with automatic IGDB matching.',
      icon: '📊',
      features: ['CSV file upload', 'Automatic IGDB matching', 'Review unmatched titles'],
      color: 'purple',
    },
  };
  return info[source];
}

// Helper to get export format display info
export function getExportFormatDisplayInfo(format: ExportFormat): {
  title: string;
  description: string;
  features: string[];
} {
  const info: Record<ExportFormat, {
    title: string;
    description: string;
    features: string[];
  }> = {
    [ExportFormat.JSON]: {
      title: 'JSON Format',
      description: 'Export your entire game collection to a JSON file for backup or transfer to another instance.',
      features: ['Complete collection backup', 'Includes all metadata', 'Recommended for re-import'],
    },
    [ExportFormat.CSV]: {
      title: 'CSV Format',
      description: 'Export your collection to a CSV file for use in spreadsheet applications.',
      features: ['Spreadsheet compatible', 'Human readable', 'Good for analysis'],
    },
  };
  return info[format];
}
