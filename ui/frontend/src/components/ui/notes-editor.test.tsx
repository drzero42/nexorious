import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import { NotesEditor, NotesViewer } from './notes-editor';

// Mock lucide-react icons to avoid rendering issues
vi.mock('lucide-react', () => ({
  Bold: () => <span data-testid="icon-bold">Bold</span>,
  Italic: () => <span data-testid="icon-italic">Italic</span>,
  Strikethrough: () => <span data-testid="icon-strikethrough">Strike</span>,
  List: () => <span data-testid="icon-list">List</span>,
  ListOrdered: () => <span data-testid="icon-list-ordered">OrderedList</span>,
  Heading1: () => <span data-testid="icon-h1">H1</span>,
  Heading2: () => <span data-testid="icon-h2">H2</span>,
  Heading3: () => <span data-testid="icon-h3">H3</span>,
  Pilcrow: () => <span data-testid="icon-pilcrow">Paragraph</span>,
}));

describe('NotesEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders editor with toolbar when editable', async () => {
    render(<NotesEditor editable={true} />);

    // Wait for editor to initialize - TipTap renders with .ProseMirror class
    await waitFor(() => {
      const editor = document.querySelector('.ProseMirror');
      expect(editor).toBeInTheDocument();
    });

    // Check that toolbar buttons are present
    expect(screen.getByTitle('Bold (Ctrl+B)')).toBeInTheDocument();
    expect(screen.getByTitle('Italic (Ctrl+I)')).toBeInTheDocument();
    expect(screen.getByTitle('Strikethrough')).toBeInTheDocument();
    expect(screen.getByTitle('Paragraph')).toBeInTheDocument();
    expect(screen.getByTitle('Heading 1')).toBeInTheDocument();
    expect(screen.getByTitle('Heading 2')).toBeInTheDocument();
    expect(screen.getByTitle('Heading 3')).toBeInTheDocument();
    expect(screen.getByTitle('Bullet list')).toBeInTheDocument();
    expect(screen.getByTitle('Numbered list')).toBeInTheDocument();
  });

  it('hides toolbar when editable is false', async () => {
    render(<NotesEditor editable={false} />);

    // Wait for editor to initialize
    await waitFor(() => {
      const editor = document.querySelector('.ProseMirror');
      expect(editor).toBeInTheDocument();
    });

    // Toolbar buttons should not be present
    expect(screen.queryByTitle('Bold (Ctrl+B)')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Italic (Ctrl+I)')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Strikethrough')).not.toBeInTheDocument();
  });

  it('updates editable state when editable prop changes', async () => {
    const { rerender } = render(<NotesEditor editable={true} />);

    await waitFor(() => {
      const editor = document.querySelector('.ProseMirror');
      expect(editor).toBeInTheDocument();
    });

    // Toolbar should be visible
    expect(screen.getByTitle('Bold (Ctrl+B)')).toBeInTheDocument();

    // Change to non-editable
    rerender(<NotesEditor editable={false} />);

    await waitFor(() => {
      // Toolbar should be hidden
      expect(screen.queryByTitle('Bold (Ctrl+B)')).not.toBeInTheDocument();
    });
  });

  it('applies custom className to wrapper', async () => {
    const customClass = 'custom-editor-class';
    const { container } = render(<NotesEditor className={customClass} />);

    await waitFor(() => {
      const wrapper = container.querySelector('.custom-editor-class');
      expect(wrapper).toBeInTheDocument();
    });
  });

  it('triggers onChange when value prop updates externally', async () => {
    const handleChange = vi.fn();
    const initialValue = '<p>Initial</p>';
    const updatedValue = '<p>Updated</p>';

    const { rerender } = render(<NotesEditor value={initialValue} onChange={handleChange} />);

    await waitFor(() => {
      const editor = document.querySelector('.ProseMirror');
      expect(editor).toBeInTheDocument();
    });

    // Wait a bit to ensure initial render is complete
    await new Promise((resolve) => setTimeout(resolve, 50));

    handleChange.mockClear();

    // Update value externally
    rerender(<NotesEditor value={updatedValue} onChange={handleChange} />);

    // Wait for the update to propagate
    await waitFor(() => {
      expect(handleChange).toHaveBeenCalled();
    });

    // TipTap's onUpdate fires when setContent is called (line 67)
    // This means external value updates will trigger onChange
    // This is the actual behavior of the component
    expect(handleChange).toHaveBeenCalledWith(updatedValue);
  });
});

describe('NotesViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('applies custom className to wrapper', async () => {
    const customClass = 'custom-viewer-class';
    const { container } = render(<NotesViewer content="<p>Content</p>" className={customClass} />);

    await waitFor(() => {
      const wrapper = container.querySelector('.custom-viewer-class');
      expect(wrapper).toBeInTheDocument();
    });
  });

  it('does not display toolbar buttons', async () => {
    const content = '<p>Content</p>';
    render(<NotesViewer content={content} />);

    await waitFor(() => {
      const viewer = document.querySelector('.ProseMirror');
      expect(viewer).toBeInTheDocument();
    });

    // No toolbar buttons should be present
    expect(screen.queryByTitle('Bold (Ctrl+B)')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Italic (Ctrl+I)')).not.toBeInTheDocument();
    expect(screen.queryByTitle('Heading 1')).not.toBeInTheDocument();
  });
});
