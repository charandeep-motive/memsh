package db

const schema = `
CREATE TABLE IF NOT EXISTS commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL UNIQUE,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_commands_last_used ON commands(last_used DESC);
CREATE INDEX IF NOT EXISTS idx_commands_frequency ON commands(frequency DESC);
`
