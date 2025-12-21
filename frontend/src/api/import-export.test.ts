import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as importExportApi from './import-export';
import { api, apiUploadFile, apiDownloadFile } from './client';

vi.mock('./client', () => ({
  api: {
    post: vi.fn(),
  },
  apiUploadFile: vi.fn(),
  apiDownloadFile: vi.fn(),
}));

describe('importExportApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('importNexoriousJson', () => {
    it('should upload file and return transformed response', async () => {
      const mockFile = new File(['{"games": []}'], 'backup.json', { type: 'application/json' });
      const mockResponse = {
        job_id: 'job-123',
        source: 'nexorious',
        status: 'pending',
        message: 'Import job created. Processing 5 games.',
        total_items: 5,
      };

      vi.mocked(apiUploadFile).mockResolvedValueOnce(mockResponse);

      const result = await importExportApi.importNexoriousJson(mockFile);

      expect(apiUploadFile).toHaveBeenCalledWith('/import/nexorious', mockFile);
      expect(result.job_id).toBe('job-123');
      expect(result.source).toBe('nexorious');
      expect(result.status).toBe('pending');
      expect(result.total_items).toBe(5);
    });
  });

  describe('exportCollectionJson', () => {
    it('should start JSON export and return job info', async () => {
      const mockResponse = {
        job_id: 'export-123',
        status: 'pending',
        message: 'Export job created. Check job status for progress.',
        estimated_items: 50,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await importExportApi.exportCollectionJson();

      expect(api.post).toHaveBeenCalledWith('/export/json');
      expect(result.job_id).toBe('export-123');
      expect(result.status).toBe('pending');
      expect(result.estimated_items).toBe(50);
    });
  });

  describe('exportCollectionCsv', () => {
    it('should start CSV export and return job info', async () => {
      const mockResponse = {
        job_id: 'export-456',
        status: 'pending',
        message: 'Export job created. Check job status for progress.',
        estimated_items: 100,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await importExportApi.exportCollectionCsv();

      expect(api.post).toHaveBeenCalledWith('/export/csv');
      expect(result.job_id).toBe('export-456');
      expect(result.estimated_items).toBe(100);
    });
  });

  describe('downloadExport', () => {
    it('should download export file and return blob with filename', async () => {
      const mockBlob = new Blob(['file content'], { type: 'application/json' });
      const mockResponse = {
        blob: mockBlob,
        filename: 'nexorious_collection_20250101.json',
      };

      vi.mocked(apiDownloadFile).mockResolvedValueOnce(mockResponse);

      const result = await importExportApi.downloadExport('export-123');

      expect(apiDownloadFile).toHaveBeenCalledWith('/export/export-123/download');
      expect(result.blob).toBe(mockBlob);
      expect(result.filename).toBe('nexorious_collection_20250101.json');
    });
  });

  describe('triggerBlobDownload', () => {
    it('should create and click a download link', () => {
      const mockBlob = new Blob(['file content'], { type: 'application/json' });
      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/mock-url');
      const mockRevokeObjectURL = vi.fn();
      const mockClick = vi.fn();
      const mockAppendChild = vi.fn();
      const mockRemoveChild = vi.fn();

      // Mock URL methods
      global.URL.createObjectURL = mockCreateObjectURL;
      global.URL.revokeObjectURL = mockRevokeObjectURL;

      // Mock document methods
      const mockAnchor = {
        href: '',
        download: '',
        click: mockClick,
      };
      vi.spyOn(document, 'createElement').mockReturnValue(mockAnchor as unknown as HTMLAnchorElement);
      vi.spyOn(document.body, 'appendChild').mockImplementation(mockAppendChild);
      vi.spyOn(document.body, 'removeChild').mockImplementation(mockRemoveChild);

      importExportApi.triggerBlobDownload(mockBlob, 'test-file.json');

      expect(mockCreateObjectURL).toHaveBeenCalledWith(mockBlob);
      expect(mockAnchor.href).toBe('blob:http://localhost/mock-url');
      expect(mockAnchor.download).toBe('test-file.json');
      expect(mockClick).toHaveBeenCalled();
      expect(mockAppendChild).toHaveBeenCalled();
      expect(mockRemoveChild).toHaveBeenCalled();
      expect(mockRevokeObjectURL).toHaveBeenCalledWith('blob:http://localhost/mock-url');
    });
  });
});
