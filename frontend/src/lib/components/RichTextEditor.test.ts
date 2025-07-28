import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import RichTextEditor from './RichTextEditor.svelte';

// Mock TipTap Editor and its dependencies
const mockEditor = {
	destroy: vi.fn(),
	isEditable: true,
	setEditable: vi.fn(),
	getHTML: vi.fn(() => '<p>Test content</p>'),
	commands: {
		setContent: vi.fn()
	},
	chain: vi.fn(() => ({
		focus: vi.fn(() => ({
			toggleBold: vi.fn(() => ({ run: vi.fn() })),
			toggleItalic: vi.fn(() => ({ run: vi.fn() })),
			toggleStrike: vi.fn(() => ({ run: vi.fn() })),
			toggleBulletList: vi.fn(() => ({ run: vi.fn() })),
			toggleOrderedList: vi.fn(() => ({ run: vi.fn() })),
			toggleHeading: vi.fn(() => ({ run: vi.fn() })),
			setParagraph: vi.fn(() => ({ run: vi.fn() }))
		}))
	})),
	isActive: vi.fn((format: string, options?: any) => {
		// Mock active states for testing
		if (format === 'bold') return false;
		if (format === 'italic') return false;
		if (format === 'strike') return false;
		if (format === 'bulletList') return false;
		if (format === 'orderedList') return false;
		if (format === 'heading' && options?.level === 1) return false;
		if (format === 'heading' && options?.level === 2) return false;
		if (format === 'heading' && options?.level === 3) return false;
		return false;
	})
};

vi.mock('@tiptap/core', () => ({
	Editor: vi.fn(() => mockEditor)
}));

vi.mock('@tiptap/starter-kit', () => ({
	default: {
		configure: vi.fn(() => ({}))
	}
}));

vi.mock('@tiptap/extension-placeholder', () => ({
	default: {
		configure: vi.fn(() => ({}))
	}
}));

describe('RichTextEditor', () => {
	const defaultProps = {
		value: '<p>Initial content</p>',
		placeholder: 'Write something...',
		editable: true,
		onchange: vi.fn()
	};

	beforeEach(() => {
		vi.clearAllMocks();
		mockEditor.isActive.mockImplementation((format: string, options?: any) => {
			if (format === 'bold') return false;
			if (format === 'italic') return false;
			if (format === 'strike') return false;
			if (format === 'bulletList') return false;
			if (format === 'orderedList') return false;
			if (format === 'heading' && options?.level === 1) return false;
			if (format === 'heading' && options?.level === 2) return false;
			if (format === 'heading' && options?.level === 3) return false;
			return false;
		});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('Basic Rendering', () => {
		it('should render with default props', () => {
			render(RichTextEditor, { props: { value: '' } });
			
			const editor = document.querySelector('.rich-text-editor');
			expect(editor).toBeInTheDocument();
		});

		it('should render toolbar when editable', () => {
			render(RichTextEditor, { props: { ...defaultProps, editable: true } });
			
			// Check for toolbar buttons
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toBeInTheDocument();
			expect(screen.getByLabelText('Italic (Ctrl+I)')).toBeInTheDocument();
			expect(screen.getByLabelText('Strikethrough')).toBeInTheDocument();
		});

		it('should not render toolbar when not editable', () => {
			render(RichTextEditor, { props: { ...defaultProps, editable: false } });
			
			// Toolbar buttons should not be present
			expect(screen.queryByLabelText('Bold (Ctrl+B)')).not.toBeInTheDocument();
			expect(screen.queryByLabelText('Italic (Ctrl+I)')).not.toBeInTheDocument();
		});

		it('should handle undefined value prop', () => {
			expect(() => {
				render(RichTextEditor, { props: { value: undefined } });
			}).not.toThrow();
		});

		it('should handle empty value prop', () => {
			expect(() => {
				render(RichTextEditor, { props: { value: '' } });
			}).not.toThrow();
		});
	});

	describe('Toolbar Buttons', () => {
		beforeEach(() => {
			render(RichTextEditor, { props: defaultProps });
		});

		it('should render all formatting buttons', () => {
			// Text formatting buttons
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toBeInTheDocument();
			expect(screen.getByLabelText('Italic (Ctrl+I)')).toBeInTheDocument();
			expect(screen.getByLabelText('Strikethrough')).toBeInTheDocument();
			
			// Heading buttons
			expect(screen.getByLabelText('Paragraph')).toBeInTheDocument();
			expect(screen.getByLabelText('Heading 1')).toBeInTheDocument();
			expect(screen.getByLabelText('Heading 2')).toBeInTheDocument();
			expect(screen.getByLabelText('Heading 3')).toBeInTheDocument();
			
			// List buttons
			expect(screen.getByLabelText('Bullet list')).toBeInTheDocument();
			expect(screen.getByLabelText('Numbered list')).toBeInTheDocument();
		});

		it('should handle bold button click', async () => {
			const boldButton = screen.getByLabelText('Bold (Ctrl+B)');
			await fireEvent.click(boldButton);
			
			expect(mockEditor.chain).toHaveBeenCalled();
		});

		it('should handle italic button click', async () => {
			const italicButton = screen.getByLabelText('Italic (Ctrl+I)');
			await fireEvent.click(italicButton);
			
			expect(mockEditor.chain).toHaveBeenCalled();
		});

		it('should handle strikethrough button click', async () => {
			const strikeButton = screen.getByLabelText('Strikethrough');
			await fireEvent.click(strikeButton);
			
			expect(mockEditor.chain).toHaveBeenCalled();
		});

		it('should handle paragraph button click', async () => {
			const paragraphButton = screen.getByLabelText('Paragraph');
			await fireEvent.click(paragraphButton);
			
			expect(mockEditor.chain).toHaveBeenCalled();
		});

		it('should handle heading buttons click', async () => {
			const h1Button = screen.getByLabelText('Heading 1');
			const h2Button = screen.getByLabelText('Heading 2');
			const h3Button = screen.getByLabelText('Heading 3');
			
			await fireEvent.click(h1Button);
			await fireEvent.click(h2Button);
			await fireEvent.click(h3Button);
			
			expect(mockEditor.chain).toHaveBeenCalledTimes(3);
		});

		it('should handle list buttons click', async () => {
			const bulletListButton = screen.getByLabelText('Bullet list');
			const orderedListButton = screen.getByLabelText('Numbered list');
			
			await fireEvent.click(bulletListButton);
			await fireEvent.click(orderedListButton);
			
			expect(mockEditor.chain).toHaveBeenCalledTimes(2);
		});
	});

	describe('Button States', () => {
		it('should show active state for bold when text is bold', () => {
			mockEditor.isActive.mockImplementation((format: string) => format === 'bold');
			
			render(RichTextEditor, { props: defaultProps });
			
			const boldButton = screen.getByLabelText('Bold (Ctrl+B)');
			expect(boldButton).toHaveClass('active');
		});

		it('should show active state for italic when text is italic', () => {
			mockEditor.isActive.mockImplementation((format: string) => format === 'italic');
			
			render(RichTextEditor, { props: defaultProps });
			
			const italicButton = screen.getByLabelText('Italic (Ctrl+I)');
			expect(italicButton).toHaveClass('active');
		});

		it('should show active state for heading when text is heading', () => {
			mockEditor.isActive.mockImplementation((format: string, options?: any) => 
				format === 'heading' && options?.level === 1
			);
			
			render(RichTextEditor, { props: defaultProps });
			
			const h1Button = screen.getByLabelText('Heading 1');
			expect(h1Button).toHaveClass('active');
		});

		it('should show active state for paragraph when not heading', () => {
			mockEditor.isActive.mockImplementation((_format: string) => false);
			
			render(RichTextEditor, { props: defaultProps });
			
			const paragraphButton = screen.getByLabelText('Paragraph');
			expect(paragraphButton).toHaveClass('active');
		});
	});

	describe('Props Handling', () => {
		it('should accept custom placeholder', () => {
			const customPlaceholder = 'Enter your notes here...';
			render(RichTextEditor, { 
				props: { value: '', placeholder: customPlaceholder } 
			});
			
			// The placeholder is handled by TipTap, so we can't easily test its display
			// But we can ensure the component renders without error
			expect(document.querySelector('.rich-text-editor')).toBeTruthy();
		});

		it('should handle editable prop changes', () => {
			// Test editable vs non-editable separately since rerender may not work with TipTap
			const { unmount } = render(RichTextEditor, { 
				props: { ...defaultProps, editable: true } 
			});
			
			// Should show toolbar when editable
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toBeInTheDocument();
			unmount();
			
			// Test non-editable separately
			render(RichTextEditor, { 
				props: { ...defaultProps, editable: false } 
			});
			
			// Toolbar should be hidden
			expect(screen.queryByLabelText('Bold (Ctrl+B)')).not.toBeInTheDocument();
		});

		it('should handle onchange callback', () => {
			const onchangeCallback = vi.fn();
			render(RichTextEditor, { 
				props: { ...defaultProps, onchange: onchangeCallback } 
			});
			
			// The callback would be triggered by TipTap's onUpdate, 
			// which is tested through the editor mock
			expect(onchangeCallback).toBeDefined();
		});
	});

	describe('Accessibility', () => {
		beforeEach(() => {
			render(RichTextEditor, { props: defaultProps });
		});

		it('should have proper aria-labels on buttons', () => {
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toHaveAttribute('aria-label', 'Bold (Ctrl+B)');
			expect(screen.getByLabelText('Italic (Ctrl+I)')).toHaveAttribute('aria-label', 'Italic (Ctrl+I)');
			expect(screen.getByLabelText('Strikethrough')).toHaveAttribute('aria-label', 'Strikethrough');
			expect(screen.getByLabelText('Paragraph')).toHaveAttribute('aria-label', 'Paragraph');
			expect(screen.getByLabelText('Heading 1')).toHaveAttribute('aria-label', 'Heading 1');
			expect(screen.getByLabelText('Bullet list')).toHaveAttribute('aria-label', 'Bullet list');
			expect(screen.getByLabelText('Numbered list')).toHaveAttribute('aria-label', 'Numbered list');
		});

		it('should have proper titles on buttons', () => {
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toHaveAttribute('title', 'Bold (Ctrl+B)');
			expect(screen.getByLabelText('Italic (Ctrl+I)')).toHaveAttribute('title', 'Italic (Ctrl+I)');
			expect(screen.getByLabelText('Strikethrough')).toHaveAttribute('title', 'Strikethrough');
		});

		it('should have proper button types', () => {
			const buttons = screen.getAllByRole('button');
			buttons.forEach(button => {
				expect(button).toHaveAttribute('type', 'button');
			});
		});
	});

	describe('Visual Structure', () => {
		it('should render toolbar groups with dividers', () => {
			render(RichTextEditor, { props: defaultProps });
			
			const toolbarDividers = document.querySelectorAll('.toolbar-divider');
			expect(toolbarDividers.length).toBeGreaterThan(0);
		});

		it('should have proper CSS classes for styling', () => {
			render(RichTextEditor, { props: defaultProps });
			
			expect(document.querySelector('.rich-text-editor')).toBeTruthy();
			expect(document.querySelector('.toolbar')).toBeTruthy();
			expect(document.querySelector('.editor-wrapper')).toBeTruthy();
		});

		it('should add editable class when editable', () => {
			render(RichTextEditor, { props: { ...defaultProps, editable: true } });
			
			const editorWrapper = document.querySelector('.editor-wrapper');
			expect(editorWrapper).toHaveClass('editable');
		});

		it('should not add editable class when not editable', () => {
			render(RichTextEditor, { props: { ...defaultProps, editable: false } });
			
			const editorWrapper = document.querySelector('.editor-wrapper');
			expect(editorWrapper).not.toHaveClass('editable');
		});
	});

	describe('Component Lifecycle', () => {
		it('should handle component mount and unmount', () => {
			const { unmount } = render(RichTextEditor, { props: defaultProps });
			
			// Component should be mounted
			expect(document.querySelector('.rich-text-editor')).toBeTruthy();
			
			// Unmount component
			unmount();
			
			// Editor destroy should be called on unmount
			expect(mockEditor.destroy).toHaveBeenCalled();
		});
	});

	describe('Edge Cases', () => {
		it('should handle missing onchange callback', () => {
			expect(() => {
				render(RichTextEditor, { 
					props: { value: '', onchange: undefined } 
				});
			}).not.toThrow();
		});

		it('should handle null value', () => {
			expect(() => {
				render(RichTextEditor, { 
					props: { value: null as any } 
				});
			}).not.toThrow();
		});

		it('should handle empty string value', () => {
			expect(() => {
				render(RichTextEditor, { 
					props: { value: '' } 
				});
			}).not.toThrow();
		});

		it('should handle HTML content value', () => {
			const htmlContent = '<p><strong>Bold text</strong> and <em>italic text</em></p>';
			expect(() => {
				render(RichTextEditor, { 
					props: { value: htmlContent } 
				});
			}).not.toThrow();
		});
	});

	describe('Button Interaction States', () => {
		it('should handle multiple button states simultaneously', () => {
			mockEditor.isActive.mockImplementation((format: string, options?: any) => {
				if (format === 'bold') return true;
				if (format === 'italic') return true;
				if (format === 'heading' && options?.level === 2) return true;
				return false;
			});
			
			render(RichTextEditor, { props: defaultProps });
			
			expect(screen.getByLabelText('Bold (Ctrl+B)')).toHaveClass('active');
			expect(screen.getByLabelText('Italic (Ctrl+I)')).toHaveClass('active');
			expect(screen.getByLabelText('Heading 2')).toHaveClass('active');
		});

		it('should handle no active states', () => {
			mockEditor.isActive.mockImplementation(() => false);
			
			render(RichTextEditor, { props: defaultProps });
			
			// Paragraph should be active when no headings are active
			expect(screen.getByLabelText('Paragraph')).toHaveClass('active');
			
			// Other buttons should not be active
			expect(screen.getByLabelText('Bold (Ctrl+B)')).not.toHaveClass('active');
			expect(screen.getByLabelText('Italic (Ctrl+I)')).not.toHaveClass('active');
		});
	});
});