import { useState, useEffect, useMemo, Fragment } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { useAdminEvents } from '@/hooks/use-events';
import { useEventTypes } from '@/hooks/use-notifications';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Activity, ChevronDown, ChevronRight } from 'lucide-react';
import { dayRangeToUTC, isRangeInverted } from '@/lib/date-range';
import type { AdminEventFilters } from '@/types';
import { useDateFormat } from '@/hooks';

export const Route = createFileRoute('/_authenticated/admin/activity/')({
  head: () => ({ meta: [{ title: 'Activity | Nexorious' }] }),
  component: AdminActivityPage,
});

const ALL = 'all';

function AdminActivityPage() {
  const { formatDateTime } = useDateFormat();
  const navigate = useNavigate();
  const { user: currentUser } = useAuth();

  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      navigate({ to: '/dashboard', replace: true });
    }
  }, [currentUser, navigate]);

  const [category, setCategory] = useState<string>(ALL);
  const [scope, setScope] = useState<string>(ALL);
  const [user, setUser] = useState<string>('');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [typeFilter, setTypeFilter] = useState<string>(ALL);
  const [since, setSince] = useState<string>('');
  const [until, setUntil] = useState<string>('');

  const { data: eventTypes } = useEventTypes();
  const categories = useMemo(() => {
    const set = new Set<string>();
    for (const m of eventTypes ?? []) set.add(m.category);
    return Array.from(set);
  }, [eventTypes]);

  const typeOptions = useMemo(() => {
    const all = eventTypes ?? [];
    return category === ALL ? all : all.filter((m) => m.category === category);
  }, [eventTypes, category]);

  const filters: AdminEventFilters = useMemo(() => {
    const range = dayRangeToUTC(since, until);
    return {
      type: typeFilter === ALL ? undefined : typeFilter,
      category: typeFilter === ALL && category !== ALL ? category : undefined,
      scope: scope === ALL ? undefined : (scope as 'user' | 'admin'),
      user: user.trim() || undefined,
      since: range.since,
      until: range.until,
    };
  }, [typeFilter, category, scope, user, since, until]);

  const rangeInverted = isRangeInverted(since, until);

  const { data, isLoading, isError, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useAdminEvents(filters, !rangeInverted);

  const events = useMemo(() => (data?.pages ?? []).flatMap((p) => p.events), [data]);

  return (
    <div className="container mx-auto p-6 space-y-6">
      <div className="flex items-center gap-2">
        <Activity className="h-6 w-6" />
        <h1 className="text-2xl font-bold">Activity</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>System events</CardTitle>
          <CardDescription>Recent activity across the system, newest first.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap gap-3">
            <Select
              value={category}
              onValueChange={(v) => {
                setCategory(v);
                setTypeFilter(ALL);
                setExpanded(null);
              }}
            >
              <SelectTrigger className="w-48">
                <SelectValue placeholder="Category" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All categories</SelectItem>
                {categories.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={typeFilter}
              onValueChange={(v) => {
                setTypeFilter(v);
                setExpanded(null);
              }}
            >
              <SelectTrigger className="w-56">
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All types</SelectItem>
                {typeOptions.map((m) => (
                  <SelectItem key={m.type} value={m.type}>
                    {m.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={scope}
              onValueChange={(v) => {
                setScope(v);
                setExpanded(null);
              }}
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Scope" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All scopes</SelectItem>
                <SelectItem value="user">User</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
              </SelectContent>
            </Select>

            <Input
              className="w-56"
              placeholder="Filter by user…"
              value={user}
              onChange={(e) => {
                setUser(e.target.value);
                setExpanded(null);
              }}
            />
            <div
              role="group"
              aria-labelledby="activity-date-range-label"
              className="flex items-center gap-2 rounded-md border px-3"
            >
              <span
                id="activity-date-range-label"
                className="text-sm text-muted-foreground whitespace-nowrap"
              >
                Date range
              </span>
              <Label htmlFor="activity-since" className="text-sm">
                From
              </Label>
              <Input
                id="activity-since"
                type="date"
                aria-label="From date"
                className="w-36 border-0 shadow-none focus-visible:ring-0"
                value={since}
                max={until || undefined}
                onChange={(e) => {
                  setSince(e.target.value);
                  setExpanded(null);
                }}
              />
              <span aria-hidden="true" className="text-muted-foreground">
                –
              </span>
              <Label htmlFor="activity-until" className="text-sm">
                To
              </Label>
              <Input
                id="activity-until"
                type="date"
                aria-label="To date"
                className="w-36 border-0 shadow-none focus-visible:ring-0"
                value={until}
                min={since || undefined}
                onChange={(e) => {
                  setUntil(e.target.value);
                  setExpanded(null);
                }}
              />
            </div>
          </div>

          {rangeInverted ? (
            <p className="text-sm text-destructive">The “To” date is before the “From” date.</p>
          ) : isError ? (
            <p className="text-sm text-destructive">Failed to load events.</p>
          ) : isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : events.length === 0 ? (
            <p className="text-sm text-muted-foreground">No events match these filters.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-8" />
                  <TableHead>Type</TableHead>
                  <TableHead>When</TableHead>
                  <TableHead>User</TableHead>
                  <TableHead>Detail</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((e) => (
                  <Fragment key={e.id}>
                    <TableRow
                      className="cursor-pointer"
                      onClick={() => setExpanded(expanded === e.id ? null : e.id)}
                    >
                      <TableCell>
                        {expanded === e.id ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col gap-1">
                          <span className="font-medium">{e.title}</span>
                          {e.category && (
                            <Badge variant="secondary" className="w-fit">
                              {e.category}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell title={e.occurredAt}>{formatDateTime(e.occurredAt)}</TableCell>
                      <TableCell>{e.actorUsername ?? '—'}</TableCell>
                      <TableCell className="max-w-md truncate">{e.body}</TableCell>
                    </TableRow>
                    {expanded === e.id && (
                      <TableRow>
                        <TableCell colSpan={5} className="bg-muted/40">
                          <div className="space-y-2">
                            <pre className="whitespace-pre-wrap text-sm">{e.body}</pre>
                            <pre className="overflow-x-auto rounded bg-background p-2 text-xs">
                              {JSON.stringify(e.payload, null, 2)}
                            </pre>
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                ))}
              </TableBody>
            </Table>
          )}

          {hasNextPage && (
            <Button variant="outline" onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
              {isFetchingNextPage ? 'Loading…' : 'Load more'}
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
