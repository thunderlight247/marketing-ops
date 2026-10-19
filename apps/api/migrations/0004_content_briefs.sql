-- Milestone 4: content planning and AI-assisted drafting.
--
-- The editable brief (excerpt, body, channel-specific copy, CTA) belongs to
-- the immutable version snapshots, so every save of a draft produces a new
-- numbered version and history is preserved. The planning record (the item)
-- owns the suggested publish slot, which the calendar reads.
ALTER TABLE content_versions
    ADD COLUMN call_to_action text,
    ADD COLUMN channel_copy jsonb NOT NULL DEFAULT '{}'
        CHECK (jsonb_typeof(channel_copy) = 'object');

ALTER TABLE content_items
    ADD COLUMN planned_publish_at timestamptz;
