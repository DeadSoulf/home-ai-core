ALTER TABLE nas_pools
ADD COLUMN reserve_percent INTEGER NOT NULL DEFAULT 5
CHECK (reserve_percent BETWEEN 0 AND 50);

ALTER TABLE nas_pools
ADD COLUMN warning_percent INTEGER NOT NULL DEFAULT 10
CHECK (warning_percent BETWEEN 0 AND 95);
