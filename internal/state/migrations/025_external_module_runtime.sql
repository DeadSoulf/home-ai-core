-- Built-in product modules are moving to independently installed Docker containers.
-- Remove only legacy registry rows. Keep historical module data/schema intact so it
-- can be migrated by the corresponding external module without destructive loss.
DELETE FROM modules WHERE id IN ('ai.agent', 'ai.cloud', 'nvr');
