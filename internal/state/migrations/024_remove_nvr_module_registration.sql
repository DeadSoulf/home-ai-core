-- Cameras / NVR is not currently a product module.
-- Remove only its Module Registry entry from upgraded installations.
-- Keep all NVR schema, permissions, storage metadata and experimental core
-- groundwork intact so a future implementation can build on it without
-- destructive data migration.
DELETE FROM modules WHERE id = 'nvr';
