/**
 * Types for import/export functionality.
 */

export interface ImportSourceInfo {
  slug: string;
  display_name: string;
  description: string;
  features: string[];
  accept: string[];
}

export enum ExportFormat {
  JSON = 'json',
  CSV = 'csv',
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

// Helper to get export format display info
export function getExportFormatDisplayInfo(format: ExportFormat): {
  title: string;
  description: string;
  features: string[];
} {
  const info: Record<
    ExportFormat,
    {
      title: string;
      description: string;
      features: string[];
    }
  > = {
    [ExportFormat.JSON]: {
      title: 'JSON Format',
      description: 'Export your entire game collection to a JSON file for backup or transfer.',
      features: ['Complete collection', 'Includes all metadata', 'Recommended for re-import'],
    },
    [ExportFormat.CSV]: {
      title: 'CSV Format',
      description: 'Export your collection to a CSV file for use in spreadsheet applications.',
      features: ['Spreadsheet compatible', 'Human readable', 'Good for analysis'],
    },
  };
  return info[format];
}
