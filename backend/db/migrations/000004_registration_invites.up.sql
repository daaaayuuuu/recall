CREATE TABLE registration_invites (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code_hash BINARY(32) NOT NULL,
    code_suffix CHAR(4) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_by_admin VARCHAR(128) NOT NULL,
    used_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    used_at DATETIME(6) NULL,
    revoked_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_registration_invites_code_hash (code_hash),
    KEY idx_registration_invites_created_id (created_at, id),
    KEY idx_registration_invites_status_created (used_at, revoked_at, created_at),
    KEY idx_registration_invites_used_by (used_by_user_id),
    CONSTRAINT chk_registration_invites_terminal_state CHECK (used_at IS NULL OR revoked_at IS NULL),
    CONSTRAINT fk_registration_invites_user FOREIGN KEY (used_by_user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
