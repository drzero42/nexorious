/**
 * Types for import mapping functionality.
 *
 * Import mappings store user preferences for how import source values
 * (e.g., "PC", "Steam") should map to platform/storefront IDs.
 */

export enum MappingType {
  PLATFORM = 'platform',
  STOREFRONT = 'storefront',
}

export interface ImportMapping {
  id: string;
  userId: string;
  importSource: string;
  mappingType: MappingType;
  sourceValue: string;
  targetId: string;
  createdAt: string;
  updatedAt: string;
}

export interface ImportMappingListResponse {
  items: ImportMapping[];
  total: number;
}

export interface CreateImportMappingRequest {
  importSource: string;
  mappingType: MappingType;
  sourceValue: string;
  targetId: string;
}

export interface BatchMappingItem {
  mappingType: MappingType;
  sourceValue: string;
  targetId: string;
}

export interface BatchImportMappingResponse {
  created: number;
  updated: number;
}
