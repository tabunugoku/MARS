-- Sign-in by user name, and API keys that can be listed and revoked.
CREATE UNIQUE INDEX users_name ON users(name COLLATE NOCASE);

ALTER TABLE api_keys ADD COLUMN id TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN last_used TEXT;
CREATE UNIQUE INDEX api_keys_id ON api_keys(id);
CREATE INDEX api_keys_user ON api_keys(user_id);

ALTER TABLE sessions ADD COLUMN remember INTEGER NOT NULL DEFAULT 0;
CREATE INDEX sessions_expiry ON sessions(expires_at);
