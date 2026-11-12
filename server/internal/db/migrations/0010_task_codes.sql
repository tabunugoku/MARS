-- Coded task texts: the API renders a task's label and error from these codes
-- in the request's language. The label and error columns keep the English text
-- (rows from before this migration keep what they stored).
ALTER TABLE tasks ADD COLUMN label_code TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN label_arg TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN error_arg TEXT NOT NULL DEFAULT '';
