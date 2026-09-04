CREATE TABLE ai_settings_versions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    version BIGINT UNSIGNED NOT NULL,
    settings_json JSON NOT NULL,
    text_api_key_ciphertext VARBINARY(8192) NULL,
    text_api_key_nonce VARBINARY(12) NULL,
    moderation_api_key_ciphertext VARBINARY(8192) NULL,
    moderation_api_key_nonce VARBINARY(12) NULL,
    image_api_key_ciphertext VARBINARY(8192) NULL,
    image_api_key_nonce VARBINARY(12) NULL,
    encryption_key_version SMALLINT UNSIGNED NOT NULL,
    created_by_admin VARCHAR(128) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_ai_settings_versions_version (version),
    CONSTRAINT chk_ai_settings_versions_version CHECK (version > 0),
    CONSTRAINT chk_ai_settings_versions_secret_pairs CHECK (
        (text_api_key_ciphertext IS NULL) = (text_api_key_nonce IS NULL) AND
        (moderation_api_key_ciphertext IS NULL) = (moderation_api_key_nonce IS NULL) AND
        (image_api_key_ciphertext IS NULL) = (image_api_key_nonce IS NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE ai_settings_current (
    singleton_id TINYINT UNSIGNED NOT NULL,
    current_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    current_settings_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    updated_at DATETIME(6) NULL,
    PRIMARY KEY (singleton_id),
    UNIQUE KEY uq_ai_settings_current_settings (current_settings_id),
    CONSTRAINT chk_ai_settings_current_singleton CHECK (singleton_id = 1),
    CONSTRAINT chk_ai_settings_current_pointer CHECK (
        (current_version = 0 AND current_settings_id IS NULL) OR
        (current_version > 0 AND current_settings_id IS NOT NULL)
    ),
    CONSTRAINT fk_ai_settings_current_version FOREIGN KEY (current_settings_id)
        REFERENCES ai_settings_versions (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO ai_settings_current (singleton_id, current_version, current_settings_id)
VALUES (1, 0, NULL);
