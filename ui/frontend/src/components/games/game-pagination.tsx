import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const PER_PAGE_OPTIONS = [25, 50, 100, 500] as const;

export interface GamesPaginationProps {
  page: number;
  perPage: number;
  totalPages: number;
  totalCount: number;
  onPageChange: (page: number) => void;
  onPerPageChange: (perPage: number) => void;
  showPerPageSelector: boolean;
}

/**
 * Compute page numbers (and ellipsis placeholders) to display.
 * Always shows: first, last, current ± 1, with 'ellipsis' where gaps exist.
 * For 7 or fewer pages shows all pages without ellipsis.
 */
function getPageRange(current: number, total: number): (number | 'ellipsis')[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }

  const pages: (number | 'ellipsis')[] = [1];

  if (current > 3) {
    pages.push('ellipsis');
  }

  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  if (current < total - 2) {
    pages.push('ellipsis');
  }

  pages.push(total);
  return pages;
}

export function GamesPagination({
  page,
  perPage,
  totalPages,
  onPageChange,
  onPerPageChange,
  showPerPageSelector,
}: GamesPaginationProps) {
  if (totalPages <= 1) return null;

  const pageRange = getPageRange(page, totalPages);
  const isFirst = page <= 1;
  const isLast = page >= totalPages;

  return (
    <div className="flex items-center justify-between">
      {showPerPageSelector ? (
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Per page</span>
          <Select
            value={String(perPage)}
            onValueChange={(v) => onPerPageChange(Number(v))}
          >
            <SelectTrigger className="w-20">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PER_PAGE_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : (
        <div />
      )}

      <Pagination>
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!isFirst) onPageChange(page - 1);
              }}
              aria-disabled={isFirst}
              className={isFirst ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
            />
          </PaginationItem>

          {pageRange.map((p, i) =>
            p === 'ellipsis' ? (
              <PaginationItem key={`ellipsis-${i}`}>
                <PaginationEllipsis />
              </PaginationItem>
            ) : (
              <PaginationItem key={p}>
                <PaginationLink
                  href="#"
                  isActive={p === page}
                  onClick={(e) => {
                    e.preventDefault();
                    onPageChange(p);
                  }}
                  className="cursor-pointer"
                >
                  {p}
                </PaginationLink>
              </PaginationItem>
            )
          )}

          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!isLast) onPageChange(page + 1);
              }}
              aria-disabled={isLast}
              className={isLast ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
