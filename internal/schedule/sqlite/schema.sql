CREATE TABLE IF NOT EXISTS schedules (
	id      TEXT PRIMARY KEY,
	rule    TEXT NOT NULL,
	action  TEXT NOT NULL,
	-- schedule.Options as JSON.
	options TEXT NOT NULL,
	enabled INTEGER NOT NULL
) STRICT;
