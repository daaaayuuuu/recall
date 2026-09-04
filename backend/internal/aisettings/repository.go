package aisettings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gamegen/backend/internal/platform/database"
)

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) Current(ctx context.Context) (Record, error) {
	var record Record
	var raw []byte
	err := repository.db.QueryRowContext(ctx, `
		SELECT v.id, v.version, v.settings_json,
		       v.text_api_key_ciphertext, v.text_api_key_nonce,
		       v.moderation_api_key_ciphertext, v.moderation_api_key_nonce,
		       v.image_api_key_ciphertext, v.image_api_key_nonce,
		       v.encryption_key_version, v.created_by_admin, v.created_at
		FROM ai_settings_current c
		JOIN ai_settings_versions v ON v.id = c.current_settings_id
		WHERE c.singleton_id = 1`).Scan(
		&record.ID, &record.Version, &raw,
		&record.TextSecret.Ciphertext, &record.TextSecret.Nonce,
		&record.ImageModerationSecret.Ciphertext, &record.ImageModerationSecret.Nonce,
		&record.ImageToImageSecret.Ciphertext, &record.ImageToImageSecret.Nonce,
		&record.EncryptionKeyVersion, &record.CreatedByAdmin, &record.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("load current AI settings: %w", err)
	}
	if err := json.Unmarshal(raw, &record.Settings); err != nil {
		return Record{}, fmt.Errorf("decode current AI settings: %w", err)
	}
	return record, nil
}

func (repository *Repository) Save(ctx context.Context, input SaveInput) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin AI settings update: %w", err)
	}
	defer tx.Rollback()

	var currentVersion int64
	if err := tx.QueryRowContext(ctx, `
		SELECT current_version FROM ai_settings_current
		WHERE singleton_id = 1 FOR UPDATE`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("lock current AI settings: %w", err)
	}
	if currentVersion != input.ExpectedVersion {
		return ErrVersionConflict
	}
	raw, err := json.Marshal(input.Record.Settings)
	if err != nil {
		return fmt.Errorf("encode AI settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_settings_versions
		(id, version, settings_json,
		 text_api_key_ciphertext, text_api_key_nonce,
		 moderation_api_key_ciphertext, moderation_api_key_nonce,
		 image_api_key_ciphertext, image_api_key_nonce,
		 encryption_key_version, created_by_admin, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Record.ID, input.Record.Version, raw,
		nullBytes(input.Record.TextSecret.Ciphertext), nullBytes(input.Record.TextSecret.Nonce),
		nullBytes(input.Record.ImageModerationSecret.Ciphertext), nullBytes(input.Record.ImageModerationSecret.Nonce),
		nullBytes(input.Record.ImageToImageSecret.Ciphertext), nullBytes(input.Record.ImageToImageSecret.Nonce),
		input.Record.EncryptionKeyVersion, input.AdminUsername, input.Record.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("insert AI settings version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ai_settings_current
		SET current_version = ?, current_settings_id = ?, updated_at = ?
		WHERE singleton_id = 1`,
		input.Record.Version, input.Record.ID, input.Record.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("publish AI settings version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_audit_logs
		(id, admin_session_id, actor_username, action, target_type, target_id, request_id, metadata, created_at)
		VALUES (?, ?, ?, 'ai_settings.published', 'ai_settings', ?, ?,
		        JSON_OBJECT('version', ?, 'previousVersion', ?), ?)`,
		input.AuditID, input.AdminSessionID, input.AdminUsername, input.Record.ID, input.RequestID,
		input.Record.Version, currentVersion, input.Record.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("audit AI settings update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit AI settings update: %w", err)
	}
	return nil
}

func nullBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
