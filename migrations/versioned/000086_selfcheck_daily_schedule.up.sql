ALTER TABLE knowledge_selfcheck_profiles
    ADD COLUMN IF NOT EXISTS daily_schedule VARCHAR(16) NOT NULL DEFAULT 'daily_0900',
    ADD COLUMN IF NOT EXISTS last_run_at TIMESTAMP WITH TIME ZONE;

