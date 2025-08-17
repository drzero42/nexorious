<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Editor } from '@tiptap/core';
  import StarterKit from '@tiptap/starter-kit';
  import Placeholder from '@tiptap/extension-placeholder';
  
  interface Props {
    value?: string | undefined;
    placeholder?: string;
    editable?: boolean;
    onchange?: ((event: CustomEvent<{ value: string }>) => void) | undefined;
  }
  
  let { 
    value = $bindable(undefined), 
    placeholder = 'Write something...', 
    editable = true, 
    onchange 
  }: Props = $props();
  
  // Handle undefined value by defaulting to empty string
  const safeValue = $derived(value ?? '');
  
  let element: HTMLElement;
  let editor: Editor | undefined;
  let editorState = $state(0); // Force reactivity updates
  
  onMount(() => {
    editor = new Editor({
      element,
      extensions: [
        StarterKit.configure({
          heading: {
            levels: [1, 2, 3]
          }
        }),
        Placeholder.configure({
          placeholder
        })
      ],
      content: safeValue,
      editable,
      onUpdate: ({ editor }: { editor: Editor }) => {
        const html = editor.getHTML();
        value = html;
        editorState++; // Trigger reactivity updates
        onchange?.(new CustomEvent('change', { detail: { value: html } }));
      },
      onSelectionUpdate: () => {
        editorState++; // Trigger reactivity updates when selection changes
      },
      editorProps: {
        attributes: {
          class: 'prose prose-sm max-w-none focus:outline-none min-h-[150px] p-3'
        }
      }
    });
  });
  
  onDestroy(() => {
    if (editor) {
      editor.destroy();
    }
  });
  
  $effect(() => {
    if (editor && editor.isEditable !== editable) {
      editor.setEditable(editable);
    }
  });
  
  $effect(() => {
    if (editor && safeValue !== editor.getHTML()) {
      editor.commands.setContent(safeValue);
    }
  });
  
  function toggleBold() {
    editor?.chain().focus().toggleBold().run();
  }
  
  function toggleItalic() {
    editor?.chain().focus().toggleItalic().run();
  }
  
  function toggleStrike() {
    editor?.chain().focus().toggleStrike().run();
  }
  
  function toggleBulletList() {
    editor?.chain().focus().toggleBulletList().run();
  }
  
  function toggleOrderedList() {
    editor?.chain().focus().toggleOrderedList().run();
  }
  
  function setHeading(level: 1 | 2 | 3) {
    editor?.chain().focus().toggleHeading({ level }).run();
  }
  
  function setParagraph() {
    editor?.chain().focus().setParagraph().run();
  }
  
  const isBold = $derived(editorState >= 0 && editor?.isActive('bold'));
  const isItalic = $derived(editorState >= 0 && editor?.isActive('italic'));
  const isStrike = $derived(editorState >= 0 && editor?.isActive('strike'));
  const isBulletList = $derived(editorState >= 0 && editor?.isActive('bulletList'));
  const isOrderedList = $derived(editorState >= 0 && editor?.isActive('orderedList'));
  const isHeading1 = $derived(editorState >= 0 && editor?.isActive('heading', { level: 1 }));
  const isHeading2 = $derived(editorState >= 0 && editor?.isActive('heading', { level: 2 }));
  const isHeading3 = $derived(editorState >= 0 && editor?.isActive('heading', { level: 3 }));
</script>

<div class="rich-text-editor">
  {#if editable}
    <div class="toolbar">
      <div class="toolbar-group">
        <button
          type="button"
          class="toolbar-button"
          class:active={isBold}
          onclick={toggleBold}
          title="Bold (Ctrl+B)"
          aria-label="Bold (Ctrl+B)"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 4h8a4 4 0 014 4 4 4 0 01-4 4H6z"></path>
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 12h9a4 4 0 014 4 4 4 0 01-4 4H6z"></path>
          </svg>
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isItalic}
          onclick={toggleItalic}
          title="Italic (Ctrl+I)"
          aria-label="Italic (Ctrl+I)"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 4h4m0 16h-4m4-16l-4 16"></path>
          </svg>
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isStrike}
          onclick={toggleStrike}
          title="Strikethrough"
          aria-label="Strikethrough"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 12h8m-8 0H4m8 0V7.5A2.5 2.5 0 019.5 5H8m4 7v4.5a2.5 2.5 0 002.5 2.5H16"></path>
          </svg>
        </button>
      </div>
      
      <div class="toolbar-divider"></div>
      
      <div class="toolbar-group">
        <button
          type="button"
          class="toolbar-button"
          class:active={!isHeading1 && !isHeading2 && !isHeading3}
          onclick={setParagraph}
          title="Paragraph"
          aria-label="Paragraph"
        >
          P
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isHeading1}
          onclick={() => setHeading(1)}
          title="Heading 1"
          aria-label="Heading 1"
        >
          H1
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isHeading2}
          onclick={() => setHeading(2)}
          title="Heading 2"
          aria-label="Heading 2"
        >
          H2
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isHeading3}
          onclick={() => setHeading(3)}
          title="Heading 3"
          aria-label="Heading 3"
        >
          H3
        </button>
      </div>
      
      <div class="toolbar-divider"></div>
      
      <div class="toolbar-group">
        <button
          type="button"
          class="toolbar-button"
          class:active={isBulletList}
          onclick={toggleBulletList}
          title="Bullet list"
          aria-label="Bullet list"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 6h13M8 12h13m-13 6h13M3 6h.01M3 12h.01M3 18h.01"></path>
          </svg>
        </button>
        <button
          type="button"
          class="toolbar-button"
          class:active={isOrderedList}
          onclick={toggleOrderedList}
          title="Numbered list"
          aria-label="Numbered list"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 20l4-16m2 16l4-16M6 9h14M4 15h14"></path>
          </svg>
        </button>
      </div>
    </div>
  {/if}
  
  <div class="editor-wrapper" class:editable>
    <div bind:this={element}></div>
  </div>
</div>

<style>
  .rich-text-editor {
    border: 1px solid #e5e7eb;
    border-radius: 0.375rem;
    overflow: hidden;
    background-color: white;
  }
  
  .toolbar {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.5rem;
    background-color: #f9fafb;
    border-bottom: 1px solid #e5e7eb;
    flex-wrap: wrap;
  }
  
  .toolbar-group {
    display: flex;
    gap: 0.25rem;
  }
  
  .toolbar-divider {
    width: 1px;
    height: 1.5rem;
    background-color: #e5e7eb;
    margin: 0 0.25rem;
  }
  
  .toolbar-button {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    padding: 0;
    border: 1px solid transparent;
    border-radius: 0.25rem;
    background-color: white;
    color: #374151;
    font-size: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease-in-out;
  }
  
  .toolbar-button:hover {
    background-color: #f3f4f6;
    border-color: #d1d5db;
  }
  
  .toolbar-button.active {
    background-color: #3b82f6;
    color: white;
    border-color: #3b82f6;
  }
  
  .toolbar-button.active:hover {
    background-color: #2563eb;
    border-color: #2563eb;
  }
  
  .editor-wrapper {
    background-color: white;
  }
  
  .editor-wrapper.editable {
    cursor: text;
  }
  
  .editor-wrapper :global(.ProseMirror) {
    min-height: 150px;
    padding: 0.75rem;
  }
  
  .editor-wrapper :global(.ProseMirror:focus) {
    outline: none;
  }
  
  .editor-wrapper :global(.ProseMirror p.is-editor-empty:first-child::before) {
    content: attr(data-placeholder);
    float: left;
    color: #9ca3af;
    pointer-events: none;
    height: 0;
  }
  
  /* Prose styles for content */
  .editor-wrapper :global(.ProseMirror h1) {
    font-size: 1.5rem;
    font-weight: 700;
    margin-top: 1rem;
    margin-bottom: 0.5rem;
  }
  
  .editor-wrapper :global(.ProseMirror h2) {
    font-size: 1.25rem;
    font-weight: 600;
    margin-top: 0.75rem;
    margin-bottom: 0.5rem;
  }
  
  .editor-wrapper :global(.ProseMirror h3) {
    font-size: 1.125rem;
    font-weight: 600;
    margin-top: 0.75rem;
    margin-bottom: 0.5rem;
  }
  
  .editor-wrapper :global(.ProseMirror p) {
    margin-bottom: 0.5rem;
    line-height: 1.625;
  }
  
  .editor-wrapper :global(.ProseMirror ul),
  .editor-wrapper :global(.ProseMirror ol) {
    padding-left: 1.5rem;
    margin-bottom: 0.5rem;
  }
  
  .editor-wrapper :global(.ProseMirror li) {
    margin-bottom: 0.25rem;
  }
  
  .editor-wrapper :global(.ProseMirror strong) {
    font-weight: 600;
  }
  
  .editor-wrapper :global(.ProseMirror em) {
    font-style: italic;
  }
  
  .editor-wrapper :global(.ProseMirror s) {
    text-decoration: line-through;
  }
</style>