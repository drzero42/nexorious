-- Re-add the preferences column with its original definition. The previous
-- contents cannot be restored (they were only ever the '{}' default).

ALTER TABLE users ADD COLUMN preferences TEXT NOT NULL DEFAULT '{}';
