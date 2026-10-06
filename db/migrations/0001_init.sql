CREATE TABLE posts (
  id         BIGSERIAL PRIMARY KEY,
  parent_id  BIGINT REFERENCES posts(id) ON DELETE SET NULL,
  message    TEXT NOT NULL,
  sage       BOOLEAN NOT NULL DEFAULT FALSE,
  ip_address TEXT,
  poster_id  TEXT NOT NULL DEFAULT '',
  last_reply BIGINT,
  tags       TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX posts_parent_activity ON posts (parent_id, updated_at DESC, id DESC);
CREATE INDEX posts_age             ON posts (created_at, id);
CREATE INDEX posts_poster_id       ON posts (poster_id, created_at DESC) WHERE poster_id <> '';

CREATE TABLE users (
  id            BIGSERIAL PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bans (
  id         BIGSERIAL PRIMARY KEY,
  ip_address TEXT NOT NULL,
  reason     TEXT,
  expires_at TIMESTAMPTZ,
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX bans_ip ON bans (ip_address);

CREATE TABLE filters (
  id          BIGSERIAL PRIMARY KEY,
  regex       TEXT NOT NULL,
  action      TEXT NOT NULL DEFAULT 'reject' CHECK (action IN ('replace', 'reject', 'ban')),
  replacement TEXT NOT NULL DEFAULT '',
  ban_days    INT NOT NULL DEFAULT 0 CHECK (ban_days >= 0),
  note        TEXT NOT NULL DEFAULT '',
  hits        BIGINT NOT NULL DEFAULT 0,
  last_hit_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
