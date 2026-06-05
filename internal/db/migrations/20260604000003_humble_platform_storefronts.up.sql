-- Associate Humble Bundle with the platforms it distributes DRM-free games for.
-- pc-linux <-> humble-bundle is already seeded in the initial migration. These
-- rows are used by both sync platform resolution and manual storefront tagging.
INSERT INTO platform_storefronts (platform, storefront) VALUES
    ('pc-windows', 'humble-bundle'),
    ('mac',        'humble-bundle'),
    ('android',    'humble-bundle')
ON CONFLICT (platform, storefront) DO NOTHING;
