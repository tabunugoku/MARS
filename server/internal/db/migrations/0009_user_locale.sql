-- The interface language a user chose: "" follows the browser, "ja" or "en".
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT '';
