'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/providers';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { toast } from 'sonner';
import {
  Gamepad2,
  Store,
  Link2,
  Search,
  Plus,
  Pencil,
  Trash2,
  Loader2,
  AlertCircle,
  RefreshCw,
} from 'lucide-react';
import * as platformsApi from '@/api/platforms';
import type { Platform, Storefront } from '@/types/platform';

type StatusFilter = 'all' | 'active' | 'inactive';
type TabValue = 'platforms' | 'storefronts' | 'associations';

interface PlatformFormData {
  name: string;
  display_name: string;
  icon_url: string;
  is_active: boolean;
  default_storefront: string;
}

interface StorefrontFormData {
  name: string;
  display_name: string;
  icon_url: string;
  base_url: string;
  is_active: boolean;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function PlatformsPageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-64" />
        <Skeleton className="h-4 w-96" />
      </div>
      <Skeleton className="h-10 w-full max-w-md" />
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="flex items-center gap-4">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-4 w-48" />
                <Skeleton className="h-6 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default function AdminPlatformsPage() {
  const router = useRouter();
  const { user: currentUser } = useAuth();

  // Data state
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [storefronts, setStorefronts] = useState<Storefront[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // UI state
  const [activeTab, setActiveTab] = useState<TabValue>('platforms');
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');

  // Platform form state
  const [showPlatformDialog, setShowPlatformDialog] = useState(false);
  const [editingPlatform, setEditingPlatform] = useState<Platform | null>(null);
  const [platformForm, setPlatformForm] = useState<PlatformFormData>({
    name: '',
    display_name: '',
    icon_url: '',
    is_active: true,
    default_storefront: '',
  });
  const [isPlatformSaving, setIsPlatformSaving] = useState(false);

  // Storefront form state
  const [showStorefrontDialog, setShowStorefrontDialog] = useState(false);
  const [editingStorefront, setEditingStorefront] = useState<Storefront | null>(null);
  const [storefrontForm, setStorefrontForm] = useState<StorefrontFormData>({
    name: '',
    display_name: '',
    icon_url: '',
    base_url: '',
    is_active: true,
  });
  const [isStorefrontSaving, setIsStorefrontSaving] = useState(false);

  // Delete confirmation state
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{
    type: 'platform' | 'storefront';
    id: string;
    name: string;
  } | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // Association state
  const [associations, setAssociations] = useState<Map<string, Set<string>>>(new Map());
  const [isLoadingAssociations, setIsLoadingAssociations] = useState(false);
  const [togglingStatus, setTogglingStatus] = useState<Set<string>>(new Set());

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      router.replace('/dashboard');
    }
  }, [currentUser, router]);

  // Fetch data
  useEffect(() => {
    const fetchData = async () => {
      try {
        setIsLoading(true);
        setError(null);
        const [platformsRes, storefrontsRes] = await Promise.all([
          platformsApi.getPlatforms({ activeOnly: false }),
          platformsApi.getStorefronts({ activeOnly: false }),
        ]);
        setPlatforms(platformsRes.platforms);
        setStorefronts(storefrontsRes.storefronts);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to load data';
        setError(errorMessage);
      } finally {
        setIsLoading(false);
      }
    };

    if (currentUser?.isAdmin) {
      fetchData();
    }
  }, [currentUser]);

  // Load associations when switching to associations tab
  useEffect(() => {
    const loadAssociations = async () => {
      if (activeTab !== 'associations' || platforms.length === 0) return;

      setIsLoadingAssociations(true);
      try {
        const newAssociations = new Map<string, Set<string>>();
        for (const platform of platforms) {
          const storefrontsList = await platformsApi.getPlatformStorefronts(
            platform.name,
            false
          );
          newAssociations.set(platform.name, new Set(storefrontsList.map((s) => s.name)));
        }
        setAssociations(newAssociations);
      } catch (err) {
        toast.error('Failed to load associations');
      } finally {
        setIsLoadingAssociations(false);
      }
    };

    loadAssociations();
  }, [activeTab, platforms]);

  // Filtered data
  const filteredPlatforms = useMemo(() => {
    return platforms.filter((p) => {
      const matchesSearch =
        p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        p.display_name.toLowerCase().includes(searchQuery.toLowerCase());
      if (statusFilter === 'active') return matchesSearch && p.is_active;
      if (statusFilter === 'inactive') return matchesSearch && !p.is_active;
      return matchesSearch;
    });
  }, [platforms, searchQuery, statusFilter]);

  const filteredStorefronts = useMemo(() => {
    return storefronts.filter((s) => {
      const matchesSearch =
        s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        s.display_name.toLowerCase().includes(searchQuery.toLowerCase());
      if (statusFilter === 'active') return matchesSearch && s.is_active;
      if (statusFilter === 'inactive') return matchesSearch && !s.is_active;
      return matchesSearch;
    });
  }, [storefronts, searchQuery, statusFilter]);

  // Platform handlers
  const handleOpenPlatformCreate = () => {
    setPlatformForm({
      name: '',
      display_name: '',
      icon_url: '',
      is_active: true,
      default_storefront: '',
    });
    setEditingPlatform(null);
    setShowPlatformDialog(true);
  };

  const handleOpenPlatformEdit = (platform: Platform) => {
    setPlatformForm({
      name: platform.name,
      display_name: platform.display_name,
      icon_url: platform.icon_url ?? '',
      is_active: platform.is_active,
      default_storefront: platform.default_storefront ?? '',
    });
    setEditingPlatform(platform);
    setShowPlatformDialog(true);
  };

  const handleSavePlatform = async () => {
    if (!platformForm.name.trim() || !platformForm.display_name.trim()) {
      toast.error('Name and display name are required');
      return;
    }

    // Convert "none" to empty string for API
    const defaultStorefront =
      platformForm.default_storefront === 'none' ? '' : platformForm.default_storefront;

    setIsPlatformSaving(true);
    try {
      if (editingPlatform) {
        const updated = await platformsApi.updatePlatform(editingPlatform.name, {
          display_name: platformForm.display_name,
          icon_url: platformForm.icon_url || null,
          is_active: platformForm.is_active,
          default_storefront: defaultStorefront || null,
        });
        setPlatforms((prev) => prev.map((p) => (p.name === updated.name ? updated : p)));
        toast.success('Platform updated successfully');
      } else {
        const created = await platformsApi.createPlatform({
          name: platformForm.name,
          display_name: platformForm.display_name,
          icon_url: platformForm.icon_url || undefined,
          is_active: platformForm.is_active,
          default_storefront: defaultStorefront || undefined,
        });
        setPlatforms((prev) => [...prev, created]);
        toast.success('Platform created successfully');
      }
      setShowPlatformDialog(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save platform';
      toast.error(message);
    } finally {
      setIsPlatformSaving(false);
    }
  };

  // Storefront handlers
  const handleOpenStorefrontCreate = () => {
    setStorefrontForm({
      name: '',
      display_name: '',
      icon_url: '',
      base_url: '',
      is_active: true,
    });
    setEditingStorefront(null);
    setShowStorefrontDialog(true);
  };

  const handleOpenStorefrontEdit = (storefront: Storefront) => {
    setStorefrontForm({
      name: storefront.name,
      display_name: storefront.display_name,
      icon_url: storefront.icon_url ?? '',
      base_url: storefront.base_url ?? '',
      is_active: storefront.is_active,
    });
    setEditingStorefront(storefront);
    setShowStorefrontDialog(true);
  };

  const handleSaveStorefront = async () => {
    if (!storefrontForm.name.trim() || !storefrontForm.display_name.trim()) {
      toast.error('Name and display name are required');
      return;
    }

    setIsStorefrontSaving(true);
    try {
      if (editingStorefront) {
        const updated = await platformsApi.updateStorefront(editingStorefront.name, {
          display_name: storefrontForm.display_name,
          icon_url: storefrontForm.icon_url || null,
          base_url: storefrontForm.base_url || null,
          is_active: storefrontForm.is_active,
        });
        setStorefronts((prev) => prev.map((s) => (s.name === updated.name ? updated : s)));
        toast.success('Storefront updated successfully');
      } else {
        const created = await platformsApi.createStorefront({
          name: storefrontForm.name,
          display_name: storefrontForm.display_name,
          icon_url: storefrontForm.icon_url || undefined,
          base_url: storefrontForm.base_url || undefined,
          is_active: storefrontForm.is_active,
        });
        setStorefronts((prev) => [...prev, created]);
        toast.success('Storefront created successfully');
      }
      setShowStorefrontDialog(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save storefront';
      toast.error(message);
    } finally {
      setIsStorefrontSaving(false);
    }
  };

  // Delete handlers
  const handleOpenDelete = (type: 'platform' | 'storefront', name: string, displayName: string) => {
    setDeleteTarget({ type, id: name, name: displayName });
    setShowDeleteDialog(true);
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;

    setIsDeleting(true);
    try {
      if (deleteTarget.type === 'platform') {
        await platformsApi.deletePlatform(deleteTarget.id);
        setPlatforms((prev) => prev.filter((p) => p.name !== deleteTarget.id));
      } else {
        await platformsApi.deleteStorefront(deleteTarget.id);
        setStorefronts((prev) => prev.filter((s) => s.name !== deleteTarget.id));
      }
      toast.success(`${deleteTarget.type === 'platform' ? 'Platform' : 'Storefront'} deleted`);
      setShowDeleteDialog(false);
      setDeleteTarget(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete';
      toast.error(message);
    } finally {
      setIsDeleting(false);
    }
  };

  // Toggle status handlers
  const handleTogglePlatformStatus = async (platform: Platform) => {
    const key = `platform-${platform.name}`;
    setTogglingStatus((prev) => new Set(prev).add(key));
    try {
      const updated = await platformsApi.updatePlatform(platform.name, {
        is_active: !platform.is_active,
      });
      setPlatforms((prev) => prev.map((p) => (p.name === updated.name ? updated : p)));
    } catch (err) {
      toast.error('Failed to toggle status');
    } finally {
      setTogglingStatus((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
    }
  };

  const handleToggleStorefrontStatus = async (storefront: Storefront) => {
    const key = `storefront-${storefront.name}`;
    setTogglingStatus((prev) => new Set(prev).add(key));
    try {
      const updated = await platformsApi.updateStorefront(storefront.name, {
        is_active: !storefront.is_active,
      });
      setStorefronts((prev) => prev.map((s) => (s.name === updated.name ? updated : s)));
    } catch (err) {
      toast.error('Failed to toggle status');
    } finally {
      setTogglingStatus((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
    }
  };

  // Association handlers
  const handleAssociationChange = async (
    platformId: string,
    storefrontId: string,
    checked: boolean
  ) => {
    try {
      if (checked) {
        await platformsApi.createPlatformStorefrontAssociation(platformId, storefrontId);
        setAssociations((prev) => {
          const next = new Map(prev);
          const set = next.get(platformId) ?? new Set();
          set.add(storefrontId);
          next.set(platformId, set);
          return next;
        });
      } else {
        await platformsApi.deletePlatformStorefrontAssociation(platformId, storefrontId);
        setAssociations((prev) => {
          const next = new Map(prev);
          const set = next.get(platformId) ?? new Set();
          set.delete(storefrontId);
          next.set(platformId, set);
          return next;
        });
      }
    } catch (err) {
      toast.error('Failed to update association');
    }
  };

  const hasAssociation = (platformId: string, storefrontId: string): boolean => {
    return associations.get(platformId)?.has(storefrontId) ?? false;
  };

  // Show nothing while checking auth
  if (!currentUser?.isAdmin) {
    return null;
  }

  if (isLoading) {
    return <PlatformsPageSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="border-b pb-5">
        <h1 className="text-3xl font-bold">Platform & Storefront Management</h1>
        <p className="mt-2 text-muted-foreground">
          Manage available platforms and storefronts for the application.
        </p>
      </div>

      {/* Info Alert */}
      <Alert>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Status Info</AlertTitle>
        <AlertDescription>
          Active platforms and storefronts are available to users when adding games. Inactive
          ones are hidden from users but preserved in the system. Click the status badges to
          toggle between active/inactive states.
        </AlertDescription>
      </Alert>

      {/* Error Alert */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription className="flex items-center justify-between">
            <span>{error}</span>
            <Button variant="outline" size="sm" onClick={() => setError(null)}>
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as TabValue)}>
        <TabsList>
          <TabsTrigger value="platforms" className="gap-2">
            <Gamepad2 className="h-4 w-4" />
            Platforms
          </TabsTrigger>
          <TabsTrigger value="storefronts" className="gap-2">
            <Store className="h-4 w-4" />
            Storefronts
          </TabsTrigger>
          <TabsTrigger value="associations" className="gap-2">
            <Link2 className="h-4 w-4" />
            Associations
          </TabsTrigger>
        </TabsList>

        {/* Search and Filter Controls */}
        <Card className="mt-4">
          <CardContent className="pt-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={`Search ${activeTab}...`}
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-9"
                />
              </div>
              <Select
                value={statusFilter}
                onValueChange={(v) => setStatusFilter(v as StatusFilter)}
              >
                <SelectTrigger className="w-full sm:w-48">
                  <SelectValue placeholder="Filter by status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="active">Active Only</SelectItem>
                  <SelectItem value="inactive">Inactive Only</SelectItem>
                </SelectContent>
              </Select>
              {activeTab !== 'associations' && (
                <Button
                  onClick={
                    activeTab === 'platforms'
                      ? handleOpenPlatformCreate
                      : handleOpenStorefrontCreate
                  }
                >
                  <Plus className="mr-2 h-4 w-4" />
                  Add {activeTab === 'platforms' ? 'Platform' : 'Storefront'}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>

        {/* Platforms Tab */}
        <TabsContent value="platforms">
          <Card>
            <CardHeader>
              <CardTitle>Platforms ({filteredPlatforms.length})</CardTitle>
              <CardDescription>
                Gaming platforms available in the system
              </CardDescription>
            </CardHeader>
            <CardContent>
              {filteredPlatforms.length === 0 ? (
                <div className="py-12 text-center text-muted-foreground">
                  No platforms found
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>Display Name</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Source</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredPlatforms.map((platform) => (
                      <TableRow key={platform.name}>
                        <TableCell className="font-medium">{platform.name}</TableCell>
                        <TableCell>{platform.display_name}</TableCell>
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-auto p-0"
                            onClick={() => handleTogglePlatformStatus(platform)}
                            disabled={togglingStatus.has(`platform-${platform.name}`)}
                          >
                            {togglingStatus.has(`platform-${platform.name}`) ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : platform.is_active ? (
                              <Badge className="bg-green-100 text-green-800 hover:bg-green-200">
                                Active
                              </Badge>
                            ) : (
                              <Badge variant="secondary">Inactive</Badge>
                            )}
                          </Button>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={platform.source === 'official' ? 'default' : 'outline'}
                          >
                            {platform.source}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {formatDate(platform.created_at)}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleOpenPlatformEdit(platform)}
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() =>
                                handleOpenDelete('platform', platform.name, platform.display_name)
                              }
                            >
                              <Trash2 className="h-4 w-4 text-destructive" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Storefronts Tab */}
        <TabsContent value="storefronts">
          <Card>
            <CardHeader>
              <CardTitle>Storefronts ({filteredStorefronts.length})</CardTitle>
              <CardDescription>
                Digital storefronts and distribution platforms
              </CardDescription>
            </CardHeader>
            <CardContent>
              {filteredStorefronts.length === 0 ? (
                <div className="py-12 text-center text-muted-foreground">
                  No storefronts found
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>Display Name</TableHead>
                      <TableHead>Base URL</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Source</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredStorefronts.map((storefront) => (
                      <TableRow key={storefront.name}>
                        <TableCell className="font-medium">{storefront.name}</TableCell>
                        <TableCell>{storefront.display_name}</TableCell>
                        <TableCell>
                          {storefront.base_url ? (
                            <a
                              href={storefront.base_url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-primary hover:underline"
                            >
                              {storefront.base_url}
                            </a>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </TableCell>
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-auto p-0"
                            onClick={() => handleToggleStorefrontStatus(storefront)}
                            disabled={togglingStatus.has(`storefront-${storefront.name}`)}
                          >
                            {togglingStatus.has(`storefront-${storefront.name}`) ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : storefront.is_active ? (
                              <Badge className="bg-green-100 text-green-800 hover:bg-green-200">
                                Active
                              </Badge>
                            ) : (
                              <Badge variant="secondary">Inactive</Badge>
                            )}
                          </Button>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={storefront.source === 'official' ? 'default' : 'outline'}
                          >
                            {storefront.source}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {formatDate(storefront.created_at)}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleOpenStorefrontEdit(storefront)}
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() =>
                                handleOpenDelete(
                                  'storefront',
                                  storefront.name,
                                  storefront.display_name
                                )
                              }
                            >
                              <Trash2 className="h-4 w-4 text-destructive" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Associations Tab */}
        <TabsContent value="associations">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Platform-Storefront Associations</CardTitle>
                <CardDescription>
                  Define which storefronts are available for each platform
                </CardDescription>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setActiveTab('associations')}
                disabled={isLoadingAssociations}
              >
                {isLoadingAssociations ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="mr-2 h-4 w-4" />
                )}
                Refresh
              </Button>
            </CardHeader>
            <CardContent>
              {isLoadingAssociations ? (
                <div className="flex justify-center py-12">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : filteredPlatforms.length === 0 || filteredStorefronts.length === 0 ? (
                <div className="py-12 text-center text-muted-foreground">
                  No platforms or storefronts available
                </div>
              ) : (
                <>
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="sticky left-0 bg-background">Platform</TableHead>
                          {filteredStorefronts.map((storefront) => (
                            <TableHead key={storefront.name} className="min-w-[120px] text-center">
                              <div className="flex flex-col items-center gap-1">
                                <span>{storefront.display_name}</span>
                                <Badge
                                  variant={storefront.is_active ? 'default' : 'secondary'}
                                  className="text-xs"
                                >
                                  {storefront.is_active ? 'Active' : 'Inactive'}
                                </Badge>
                              </div>
                            </TableHead>
                          ))}
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredPlatforms.map((platform) => (
                          <TableRow key={platform.name}>
                            <TableCell className="sticky left-0 bg-background font-medium">
                              <div className="flex flex-col gap-1">
                                <span>{platform.display_name}</span>
                                <Badge
                                  variant={platform.is_active ? 'default' : 'secondary'}
                                  className="w-fit text-xs"
                                >
                                  {platform.is_active ? 'Active' : 'Inactive'}
                                </Badge>
                              </div>
                            </TableCell>
                            {filteredStorefronts.map((storefront) => (
                              <TableCell key={storefront.name} className="text-center">
                                <Checkbox
                                  checked={hasAssociation(platform.name, storefront.name)}
                                  onCheckedChange={(checked) =>
                                    handleAssociationChange(
                                      platform.name,
                                      storefront.name,
                                      checked === true
                                    )
                                  }
                                  disabled={!platform.is_active || !storefront.is_active}
                                />
                              </TableCell>
                            ))}
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                  <p className="mt-4 text-sm text-muted-foreground">
                    <strong>Note:</strong> Checkboxes are disabled for inactive platforms or
                    storefronts. Only active items can have associations.
                  </p>
                </>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Platform Dialog */}
      <Dialog open={showPlatformDialog} onOpenChange={setShowPlatformDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editingPlatform ? 'Edit Platform' : 'Create New Platform'}
            </DialogTitle>
            <DialogDescription>
              {editingPlatform
                ? 'Update the platform details.'
                : 'Add a new gaming platform to the system.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="platform-name">Platform Name</Label>
              <Input
                id="platform-name"
                value={platformForm.name}
                onChange={(e) =>
                  setPlatformForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder="e.g., nintendo_switch"
                disabled={!!editingPlatform}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="platform-display-name">Display Name</Label>
              <Input
                id="platform-display-name"
                value={platformForm.display_name}
                onChange={(e) =>
                  setPlatformForm((prev) => ({ ...prev, display_name: e.target.value }))
                }
                placeholder="e.g., Nintendo Switch"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="platform-icon-url">Icon URL (Optional)</Label>
              <Input
                id="platform-icon-url"
                value={platformForm.icon_url}
                onChange={(e) =>
                  setPlatformForm((prev) => ({ ...prev, icon_url: e.target.value }))
                }
                placeholder="/static/logos/platforms/example/icon.svg"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="platform-default-storefront">Default Storefront (Optional)</Label>
              <Select
                value={platformForm.default_storefront}
                onValueChange={(v) =>
                  setPlatformForm((prev) => ({ ...prev, default_storefront: v }))
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="No Default" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No Default</SelectItem>
                  {storefronts
                    .filter((s) => s.is_active)
                    .map((s) => (
                      <SelectItem key={s.name} value={s.name}>
                        {s.display_name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="platform-is-active"
                checked={platformForm.is_active}
                onCheckedChange={(checked) =>
                  setPlatformForm((prev) => ({ ...prev, is_active: checked === true }))
                }
              />
              <Label htmlFor="platform-is-active">Active platform</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowPlatformDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleSavePlatform} disabled={isPlatformSaving}>
              {isPlatformSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingPlatform ? 'Update' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Storefront Dialog */}
      <Dialog open={showStorefrontDialog} onOpenChange={setShowStorefrontDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editingStorefront ? 'Edit Storefront' : 'Create New Storefront'}
            </DialogTitle>
            <DialogDescription>
              {editingStorefront
                ? 'Update the storefront details.'
                : 'Add a new digital storefront to the system.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="storefront-name">Storefront Name</Label>
              <Input
                id="storefront-name"
                value={storefrontForm.name}
                onChange={(e) =>
                  setStorefrontForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder="e.g., epic_games"
                disabled={!!editingStorefront}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="storefront-display-name">Display Name</Label>
              <Input
                id="storefront-display-name"
                value={storefrontForm.display_name}
                onChange={(e) =>
                  setStorefrontForm((prev) => ({ ...prev, display_name: e.target.value }))
                }
                placeholder="e.g., Epic Games Store"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="storefront-base-url">Base URL (Optional)</Label>
              <Input
                id="storefront-base-url"
                type="url"
                value={storefrontForm.base_url}
                onChange={(e) =>
                  setStorefrontForm((prev) => ({ ...prev, base_url: e.target.value }))
                }
                placeholder="https://store.epicgames.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="storefront-icon-url">Icon URL (Optional)</Label>
              <Input
                id="storefront-icon-url"
                value={storefrontForm.icon_url}
                onChange={(e) =>
                  setStorefrontForm((prev) => ({ ...prev, icon_url: e.target.value }))
                }
                placeholder="/static/logos/storefronts/example/icon.svg"
              />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="storefront-is-active"
                checked={storefrontForm.is_active}
                onCheckedChange={(checked) =>
                  setStorefrontForm((prev) => ({ ...prev, is_active: checked === true }))
                }
              />
              <Label htmlFor="storefront-is-active">Active storefront</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowStorefrontDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleSaveStorefront} disabled={isStorefrontSaving}>
              {isStorefrontSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingStorefront ? 'Update' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Confirm Deletion</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete the {deleteTarget?.type} &quot;{deleteTarget?.name}
              &quot;?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              <strong>Warning:</strong> This action cannot be undone. The {deleteTarget?.type}{' '}
              will be removed from all user games.
            </AlertDescription>
          </Alert>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {isDeleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
