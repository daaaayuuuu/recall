CREATE TABLE users (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    login_id VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NULL,
    avatar_asset_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    status VARCHAR(32) NOT NULL,
    last_login_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_login_id (login_id),
    KEY idx_users_status_created (status, created_at),
    KEY idx_users_created_id (created_at, id),
    CONSTRAINT chk_users_status CHECK (status IN ('active', 'disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE user_sessions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    token_hash BINARY(32) NOT NULL,
    csrf_token_hash BINARY(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NULL,
    revoked_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_user_sessions_token_hash (token_hash),
    KEY idx_user_sessions_user_revoked (user_id, revoked_at),
    KEY idx_user_sessions_expires (expires_at),
    CONSTRAINT fk_user_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE admin_sessions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    admin_username VARCHAR(128) NOT NULL,
    token_hash BINARY(32) NOT NULL,
    csrf_token_hash BINARY(32) NOT NULL,
    credential_fingerprint BINARY(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NULL,
    revoked_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_admin_sessions_token_hash (token_hash),
    KEY idx_admin_sessions_expires (expires_at),
    KEY idx_admin_sessions_username_revoked (admin_username, revoked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE assets (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    owner_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    kind VARCHAR(32) NOT NULL,
    bucket VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    object_key VARCHAR(1024) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT UNSIGNED NOT NULL,
    checksum_sha256 BINARY(32) NOT NULL,
    width INT UNSIGNED NULL,
    height INT UNSIGNED NULL,
    internal_status VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_assets_object_key (object_key),
    KEY idx_assets_owner_created (owner_user_id, created_at),
    KEY idx_assets_status_created (internal_status, created_at),
    CONSTRAINT chk_assets_kind CHECK (kind IN ('avatar', 'game_source', 'game_cover', 'game_render', 'game_artifact')),
    CONSTRAINT fk_assets_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE users
    ADD CONSTRAINT fk_users_avatar_asset FOREIGN KEY (avatar_asset_id) REFERENCES assets (id) ON DELETE SET NULL;

CREATE TABLE games (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    title VARCHAR(120) NOT NULL,
    description VARCHAR(500) NULL,
    cover_asset_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    status VARCHAR(32) NOT NULL,
    current_version_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    current_generation_run_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    deletion_requested_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_games_user_created (user_id, created_at, id),
    KEY idx_games_user_status_updated (user_id, status, updated_at),
    KEY idx_games_status_updated (status, updated_at),
    KEY idx_games_cover_asset (cover_asset_id),
    CONSTRAINT chk_games_status CHECK (status IN ('draft', 'queued', 'generating', 'ready', 'failed', 'deleting')),
    CONSTRAINT fk_games_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT,
    CONSTRAINT fk_games_cover_asset FOREIGN KEY (cover_asset_id) REFERENCES assets (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE game_versions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    version_number INT UNSIGNED NOT NULL,
    status VARCHAR(32) NOT NULL,
    input_schema_version SMALLINT UNSIGNED NOT NULL,
    input_payload_ciphertext MEDIUMBLOB NOT NULL,
    input_payload_nonce VARBINARY(12) NOT NULL,
    encryption_key_version SMALLINT UNSIGNED NOT NULL,
    template_id VARCHAR(64) NOT NULL,
    template_version VARCHAR(32) NOT NULL,
    game_config_asset_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_game_versions_number (game_id, version_number),
    KEY idx_game_versions_created (game_id, created_at),
    KEY idx_game_versions_config_asset (game_config_asset_id),
    CONSTRAINT chk_game_versions_status CHECK (status IN ('draft', 'queued', 'generating', 'ready', 'failed')),
    CONSTRAINT fk_game_versions_game FOREIGN KEY (game_id) REFERENCES games (id) ON DELETE CASCADE,
    CONSTRAINT fk_game_versions_config_asset FOREIGN KEY (game_config_asset_id) REFERENCES assets (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE game_version_assets (
    game_version_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    asset_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    role VARCHAR(32) NOT NULL,
    sort_order INT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (game_version_id, asset_id),
    KEY idx_game_version_assets_asset (asset_id),
    CONSTRAINT chk_game_version_assets_role CHECK (role IN ('source', 'cover', 'render', 'artifact')),
    CONSTRAINT fk_game_version_assets_version FOREIGN KEY (game_version_id) REFERENCES game_versions (id) ON DELETE CASCADE,
    CONSTRAINT fk_game_version_assets_asset FOREIGN KEY (asset_id) REFERENCES assets (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE game_generation_runs (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_version_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    attempt_number INT UNSIGNED NOT NULL,
    execution_count INT UNSIGNED NOT NULL DEFAULT 0,
    trigger_type VARCHAR(32) NOT NULL,
    idempotency_key_hash BINARY(32) NULL,
    status VARCHAR(32) NOT NULL,
    stage VARCHAR(32) NOT NULL,
    progress TINYINT UNSIGNED NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NULL,
    admin_message VARCHAR(500) NULL,
    sanitized_details JSON NULL,
    retryable BOOLEAN NOT NULL DEFAULT FALSE,
    trace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    lease_owner VARCHAR(128) NULL,
    lease_expires_at DATETIME(6) NULL,
    heartbeat_at DATETIME(6) NULL,
    cancel_requested_at DATETIME(6) NULL,
    next_attempt_at DATETIME(6) NOT NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_generation_runs_attempt (game_version_id, attempt_number),
    UNIQUE KEY uq_generation_runs_idempotency (game_id, idempotency_key_hash),
    KEY idx_generation_runs_queue (status, next_attempt_at, created_at),
    KEY idx_generation_runs_game_created (game_id, created_at),
    KEY idx_generation_runs_error_completed (error_code, completed_at),
    KEY idx_generation_runs_trace (trace_id),
    CONSTRAINT chk_generation_runs_trigger CHECK (trigger_type IN ('initial', 'user_retry')),
    CONSTRAINT chk_generation_runs_status CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT chk_generation_runs_progress CHECK (progress <= 100),
    CONSTRAINT fk_generation_runs_game FOREIGN KEY (game_id) REFERENCES games (id) ON DELETE CASCADE,
    CONSTRAINT fk_generation_runs_version FOREIGN KEY (game_version_id) REFERENCES game_versions (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE games
    ADD CONSTRAINT fk_games_current_version FOREIGN KEY (current_version_id) REFERENCES game_versions (id) ON DELETE SET NULL,
    ADD CONSTRAINT fk_games_current_generation_run FOREIGN KEY (current_generation_run_id) REFERENCES game_generation_runs (id) ON DELETE SET NULL;

CREATE TABLE share_links (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_version_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    public_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    secret_hash BINARY(32) NOT NULL,
    secret_ciphertext VARBINARY(512) NOT NULL,
    secret_nonce VARBINARY(12) NOT NULL,
    encryption_key_version SMALLINT UNSIGNED NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    revoke_reason VARCHAR(32) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_share_links_public_id (public_id),
    UNIQUE KEY uq_share_links_secret_hash (secret_hash),
    KEY idx_share_links_game_created (game_id, created_at),
    KEY idx_share_links_expires_revoked (expires_at, revoked_at),
    KEY idx_share_links_version (game_version_id),
    KEY idx_share_links_creator (created_by_user_id),
    CONSTRAINT chk_share_links_lifetime CHECK (expires_at > created_at AND expires_at <= created_at + INTERVAL 90 DAY),
    CONSTRAINT fk_share_links_game FOREIGN KEY (game_id) REFERENCES games (id) ON DELETE CASCADE,
    CONSTRAINT fk_share_links_version FOREIGN KEY (game_version_id) REFERENCES game_versions (id) ON DELETE CASCADE,
    CONSTRAINT fk_share_links_creator FOREIGN KEY (created_by_user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE play_sessions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    share_link_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    token_hash BINARY(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_play_sessions_token_hash (token_hash),
    KEY idx_play_sessions_share (share_link_id, expires_at),
    KEY idx_play_sessions_expires (expires_at),
    CONSTRAINT chk_play_sessions_lifetime CHECK (expires_at = created_at + INTERVAL 30 MINUTE),
    CONSTRAINT fk_play_sessions_share FOREIGN KEY (share_link_id) REFERENCES share_links (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE admin_audit_logs (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    admin_session_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    actor_username VARCHAR(128) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(32) NOT NULL,
    target_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    request_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    metadata JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_admin_audit_created (created_at, id),
    KEY idx_admin_audit_actor_created (actor_username, created_at),
    KEY idx_admin_audit_target (target_type, target_id, created_at),
    KEY idx_admin_audit_session (admin_session_id),
    CONSTRAINT fk_admin_audit_session FOREIGN KEY (admin_session_id) REFERENCES admin_sessions (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE game_deletion_jobs (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    game_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    object_prefixes JSON NOT NULL,
    status VARCHAR(32) NOT NULL,
    attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(6) NOT NULL,
    last_error_code VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    completed_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    KEY idx_game_deletion_jobs_queue (status, next_attempt_at, created_at),
    KEY idx_game_deletion_jobs_game (game_id),
    CONSTRAINT chk_game_deletion_jobs_status CHECK (status IN ('queued', 'running', 'succeeded', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
