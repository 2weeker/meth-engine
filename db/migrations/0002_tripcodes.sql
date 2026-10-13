ALTER TABLE posts ADD COLUMN name     TEXT NOT NULL DEFAULT '';
ALTER TABLE posts ADD COLUMN tripcode TEXT NOT NULL DEFAULT '';
CREATE INDEX posts_tripcode ON posts (tripcode, created_at DESC) WHERE tripcode <> '';
