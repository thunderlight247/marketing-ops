-- Migration 0020: Social post enhancements
-- Adds target_platforms (e.g. instagram, facebook, linkedin, twitter, tiktok)
-- and media_urls (image/video asset links) to content_items.
-- Both default to empty arrays so existing rows are unaffected.

ALTER TABLE content_items
    ADD COLUMN IF NOT EXISTS target_platforms text[] NOT NULL DEFAULT '{}';

ALTER TABLE content_items
    ADD COLUMN IF NOT EXISTS media_urls text[] NOT NULL DEFAULT '{}';
