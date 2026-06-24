-- Add the Telltale Games storefront as a manual-only PC (Windows) storefront.
-- Reference-data only: no sync adapter or service changes. The icon is served
-- from ui/frontend/public/logos/storefronts/telltale/.
INSERT INTO public.storefronts VALUES ('telltale', 'Telltale Games', 'telltale-icon-light.svg', 'https://telltale.com');

INSERT INTO public.platform_storefronts VALUES ('pc-windows', 'telltale');
