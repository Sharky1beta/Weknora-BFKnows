ALTER TABLE knowledge_selfcheck_profiles
    DROP COLUMN IF EXISTS last_run_at,
    DROP COLUMN IF EXISTS daily_schedule;

