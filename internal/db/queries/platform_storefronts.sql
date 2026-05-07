-- name: ListStorefrontsForPlatform :many
SELECT s.* FROM storefronts s
JOIN platform_storefronts ps ON ps.storefront = s.name
WHERE ps.platform = $1
ORDER BY s.display_name;
