package db

const schemaVersion = 2

// schema is applied to a brand-new database (no prior tables).
const schema = `
CREATE TABLE IF NOT EXISTS commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    UNIQUE(command, directory)
);

CREATE INDEX IF NOT EXISTS idx_commands_last_used ON commands(last_used DESC);
CREATE INDEX IF NOT EXISTS idx_commands_frequency ON commands(frequency DESC);

CREATE TABLE IF NOT EXISTS command_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    command     TEXT    NOT NULL,
    directory   TEXT    NOT NULL DEFAULT '',
    executed_at INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    log_file    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs(executed_at DESC);
`

// migrateToCompositeKey rebuilds a legacy table (UNIQUE on command alone).
const migrateToCompositeKey = `
CREATE TABLE commands_migrated (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    UNIQUE(command, directory)
);

INSERT INTO commands_migrated (id, command, frequency, last_used, directory, exit_code)
    SELECT id, command, frequency, last_used, directory, exit_code FROM commands;

DROP TABLE commands;
ALTER TABLE commands_migrated RENAME TO commands;

CREATE INDEX IF NOT EXISTS idx_commands_last_used ON commands(last_used DESC);
CREATE INDEX IF NOT EXISTS idx_commands_frequency ON commands(frequency DESC);
`

// migrateAddCommandLogs adds the command_logs table to a v1 database.
const migrateAddCommandLogs = `
CREATE TABLE IF NOT EXISTS command_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    command     TEXT    NOT NULL,
    directory   TEXT    NOT NULL DEFAULT '',
    executed_at INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    log_file    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs(executed_at DESC);
`
