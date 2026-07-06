package db

// schemaVersion is the current on-disk schema version tracked via
// PRAGMA user_version. Bump it whenever a migration is added.
const schemaVersion = 1

// schema is the schema for a fresh database. It is kept in sync with the
// latest migration so new installs never need to migrate.
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
`

// migrateToCompositeKey rebuilds a legacy table (UNIQUE on command alone)
// so the uniqueness constraint spans (command, directory). Every existing
// row is preserved. Rows already have a directory value, so no collisions
// occur during the copy.
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
