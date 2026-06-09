import { describe, it, expect, vi } from 'vitest';
import * as importExportApi from './import-export';

vi.mock('./client', () => ({
  api: {
    post: vi.fn(),
  },
  apiUploadFile: vi.fn(),
  apiDownloadFile: vi.fn(),
}));

describe('importExportApi', () => {
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
      vi.spyOn(document, 'createElement').mockReturnValue(
        mockAnchor as unknown as HTMLAnchorElement,
      );
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
