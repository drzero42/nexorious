import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSlug from 'rehype-slug';
import { useNavigate } from '@tanstack/react-router';
import { resolveDocHref } from '@/lib/doc-links';

interface TocEntry {
  id: string;
  text: string;
  level: 2 | 3;
}

function scrollToId(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

export function MarkdownDoc({ slug, markdown }: { slug: string; markdown: string }) {
  const navigate = useNavigate();
  const containerRef = useRef<HTMLDivElement>(null);
  const [toc, setToc] = useState<TocEntry[]>([]);

  // Build the TOC from the rendered headings so each entry's id matches exactly
  // the id rehype-slug assigned (including github-slugger de-duplication).
  useEffect(() => {
    const root = containerRef.current;
    if (!root) return;
    const entries: TocEntry[] = [];
    root.querySelectorAll('h2, h3').forEach((h) => {
      if (!h.id) return;
      entries.push({
        id: h.id,
        text: h.textContent ?? '',
        level: h.tagName === 'H2' ? 2 : 3,
      });
    });
    setToc(entries);
  }, [markdown, slug]);

  return (
    <div className="flex gap-8">
      {toc.length > 0 && (
        <nav
          aria-label="On this page"
          className="hidden lg:block w-56 shrink-0 sticky top-6 self-start text-sm"
        >
          <p className="mb-2 font-medium text-muted-foreground">On this page</p>
          <ul className="space-y-1">
            {toc.map((e) => (
              <li key={e.id} className={e.level === 3 ? 'pl-3' : ''}>
                <a
                  href={`#${e.id}`}
                  className="text-muted-foreground hover:text-foreground"
                  onClick={(ev) => {
                    ev.preventDefault();
                    scrollToId(e.id);
                  }}
                >
                  {e.text}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      )}

      <div ref={containerRef} className="prose dark:prose-invert min-w-0 max-w-none flex-1">
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          rehypePlugins={[rehypeSlug]}
          components={{
            a: ({ href, children, node, ...props }) => {
              void node;
              const resolved = resolveDocHref(href ?? '', slug);
              if (resolved.type === 'internal') {
                const to = `/help/${resolved.slug}`;
                return (
                  <a
                    href={to + (resolved.hash ?? '')}
                    onClick={(e) => {
                      e.preventDefault();
                      void navigate({
                        to,
                        hash: resolved.hash ? resolved.hash.slice(1) : undefined,
                      });
                    }}
                    {...props}
                  >
                    {children}
                  </a>
                );
              }
              if (resolved.type === 'anchor') {
                return (
                  <a
                    href={resolved.value}
                    onClick={(e) => {
                      e.preventDefault();
                      scrollToId(resolved.value.slice(1));
                    }}
                    {...props}
                  >
                    {children}
                  </a>
                );
              }
              return (
                <a href={resolved.value} target="_blank" rel="noopener noreferrer" {...props}>
                  {children}
                </a>
              );
            },
          }}
        >
          {markdown}
        </ReactMarkdown>
      </div>
    </div>
  );
}
