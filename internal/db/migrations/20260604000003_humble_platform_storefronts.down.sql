-- Remove the three Humble Bundle associations added in the up migration. The
-- pc-linux <-> humble-bundle row predates this migration and is left intact.
DELETE FROM platform_storefronts
WHERE storefront = 'humble-bundle'
  AND platform IN ('pc-windows', 'mac', 'android');
