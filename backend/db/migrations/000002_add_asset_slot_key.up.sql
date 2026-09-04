ALTER TABLE game_version_assets
    ADD COLUMN slot_key VARCHAR(64) NULL AFTER role,
    ADD KEY idx_game_version_assets_slot (game_version_id, slot_key, sort_order);
