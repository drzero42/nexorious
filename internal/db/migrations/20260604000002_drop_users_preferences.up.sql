-- Drop the vestigial users.preferences column. After #797 removed the
-- PUT /api/auth/me writer and the frontend reader, the column was only ever
-- set to its '{}' default at user creation and never consumed. See #798.

ALTER TABLE users DROP COLUMN preferences;
