# DarkadiaFileUpload Component

A sophisticated Svelte 5 file upload component designed specifically for importing CSV files from Darkadia game collections. The component provides a complete upload experience with drag-and-drop functionality, validation, progress tracking, and error handling.

## Features

- **Drag-and-drop interface** with visual feedback
- **File validation** (CSV only, max 10MB)
- **Progress tracking** for upload and processing phases
- **Automatic import triggering** after successful upload
- **Error handling** with recovery options
- **Mobile responsive** design
- **Full accessibility** support (ARIA labels, keyboard navigation)
- **Integration with Darkadia store** for state management

## Usage

```svelte
<script>
  import { DarkadiaFileUpload } from '$lib/components';
  
  function handleUploadComplete(result) {
    console.log('Upload completed:', result);
    // Handle successful upload
  }
  
  function handleUploadError(error) {
    console.error('Upload failed:', error);
    // Handle upload error
  }
</script>

<DarkadiaFileUpload
  onUploadComplete={handleUploadComplete}
  onUploadError={handleUploadError}
/>
```

## Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `onUploadComplete` | `(result: DarkadiaUploadResponse) => void` | `undefined` | Callback fired when upload completes successfully |
| `onUploadStart` | `() => void` | `undefined` | Callback fired when upload begins |
| `onUploadError` | `(error: string) => void` | `undefined` | Callback fired when upload fails |
| `disabled` | `boolean` | `false` | Disables the upload functionality |
| `class` | `string` | `''` | Additional CSS classes for styling |

## Upload States

The component handles the following states automatically:

- **idle** - Ready to accept files
- **dragging** - File being dragged over drop zone
- **uploading** - File being uploaded to server
- **processing** - Server processing the uploaded CSV
- **success** - Upload and processing complete
- **error** - Upload or processing failed

## File Validation

- **File Type**: Only CSV files (`.csv`, `text/csv`, `application/csv`)
- **File Size**: Maximum 10MB
- **File Content**: Must not be empty

## Integration

The component integrates seamlessly with the Darkadia store:

```javascript
// The component automatically calls:
await darkadia.uploadCSV(file)

// Which:
// 1. Uploads the file to the server
// 2. Auto-triggers the import process
// 3. Polls for completion status
// 4. Updates reactive state throughout
```

## Styling

The component uses Tailwind CSS classes and follows the existing design system. Key styling features:

- Responsive design (mobile-first)
- Visual state indicators (drag, upload, error, success)
- Smooth transitions and animations
- Consistent with other upload components

## Accessibility

- Full keyboard navigation support
- Proper ARIA labels and roles
- Screen reader friendly
- Focus management
- High contrast support

## Error Handling

The component provides user-friendly error messages for:

- Invalid file types
- Files too large
- Empty files
- Network errors
- Server processing errors

Users can retry uploads or choose different files after errors.

## Example Implementation

See `/routes/import/darkadia/+page.svelte` for a complete implementation example showing:

- Component integration
- Callback handling
- Success state management
- Navigation after upload
- User instructions and guidance

## Testing

The component includes comprehensive tests covering:

- Initial render state
- File validation
- Drag and drop functionality
- Props and callbacks
- Visual states and accessibility
- Error handling

Run tests with:
```bash
npm test DarkadiaFileUpload.test.ts
```