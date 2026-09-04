package sharing

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gamegen/backend/internal/platform/database"
)

var (
	ErrNotFound     = errors.New("share not found")
	ErrGameNotReady = errors.New("game is not ready to share")
	ErrShareEnded   = errors.New("share has ended")
	ErrPlayExpired  = errors.New("play session has expired")
)

type Share struct {
	ID                   string
	GameID               string
	GameVersionID        string
	CreatedByUserID      string
	PublicID             string
	SecretHash           []byte
	SecretCiphertext     []byte
	SecretNonce          []byte
	EncryptionKeyVersion int
	ExpiresAt            time.Time
	RevokedAt            sql.NullTime
	RevokeReason         sql.NullString
	CreatedAt            time.Time
	GameTitle            string
	GameStatus           string
	VersionStatus        string
	CreatorNickname      sql.NullString
}

type PlaySession struct {
	ID                string
	ShareLinkID       string
	ExpiresAt         time.Time
	GameID            string
	GameVersionID     string
	GameTitle         string
	TemplateID        string
	TemplateVersion   string
	ConfigAssetID     string
	ConfigBucket      string
	ConfigObjectKey   string
	CreatorNickname   sql.NullString
	ShareExpiresAt    time.Time
	ShareRevokedAt    sql.NullTime
	GameStatus        string
	GameVersionStatus string
}

type RenderAsset struct {
	ID        string
	Role      string
	MIMEType  string
	Bucket    string
	ObjectKey string
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) CurrentVersion(ctx context.Context, userID, gameID string) (string, error) {
	var versionID sql.NullString
	if err := repository.db.QueryRowContext(ctx, `
		SELECT current_version_id FROM games
		WHERE id = ? AND user_id = ? AND status <> 'deleting'`, gameID, userID).Scan(&versionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("select current share version: %w", err)
	}
	if !versionID.Valid {
		return "", ErrGameNotReady
	}
	return versionID.String, nil
}

func (repository *Repository) CreateShare(ctx context.Context, share Share, userID string, now time.Time) (Share, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Share{}, fmt.Errorf("begin share creation: %w", err)
	}
	defer tx.Rollback()

	var currentVersionID sql.NullString
	var gameStatus, versionStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT g.current_version_id, g.status, v.status
		FROM games g
		LEFT JOIN game_versions v ON v.id = g.current_version_id
		WHERE g.id = ? AND g.user_id = ? AND g.status <> 'deleting' FOR UPDATE`,
		share.GameID, userID,
	).Scan(&currentVersionID, &gameStatus, &versionStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Share{}, ErrNotFound
		}
		return Share{}, fmt.Errorf("lock game for share: %w", err)
	}
	if !currentVersionID.Valid || currentVersionID.String != share.GameVersionID || gameStatus != "ready" || versionStatus != "ready" {
		return Share{}, ErrGameNotReady
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO share_links
		(id, game_id, game_version_id, created_by_user_id, public_id, secret_hash,
		 secret_ciphertext, secret_nonce, encryption_key_version, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		share.ID, share.GameID, share.GameVersionID, userID, share.PublicID, share.SecretHash,
		share.SecretCiphertext, share.SecretNonce, share.EncryptionKeyVersion, share.ExpiresAt.UTC(), now.UTC())
	if err != nil {
		return Share{}, fmt.Errorf("insert share link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Share{}, fmt.Errorf("commit share creation: %w", err)
	}
	return repository.GetShare(ctx, userID, share.GameID, share.ID)
}

const shareColumns = `
	s.id, s.game_id, s.game_version_id, s.created_by_user_id, s.public_id,
	s.secret_hash, s.secret_ciphertext, s.secret_nonce, s.encryption_key_version,
	s.expires_at, s.revoked_at, s.revoke_reason, s.created_at,
	g.title, g.status, v.status, u.nickname`

type scanner interface {
	Scan(...any) error
}

func scanShare(row scanner) (Share, error) {
	var share Share
	err := row.Scan(
		&share.ID, &share.GameID, &share.GameVersionID, &share.CreatedByUserID, &share.PublicID,
		&share.SecretHash, &share.SecretCiphertext, &share.SecretNonce, &share.EncryptionKeyVersion,
		&share.ExpiresAt, &share.RevokedAt, &share.RevokeReason, &share.CreatedAt,
		&share.GameTitle, &share.GameStatus, &share.VersionStatus, &share.CreatorNickname,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	if err != nil {
		return Share{}, fmt.Errorf("scan share: %w", err)
	}
	return share, nil
}

const shareFrom = `
	FROM share_links s
	JOIN games g ON g.id = s.game_id
	JOIN game_versions v ON v.id = s.game_version_id
	JOIN users u ON u.id = s.created_by_user_id `

func (repository *Repository) GetShare(ctx context.Context, userID, gameID, shareID string) (Share, error) {
	return scanShare(repository.db.QueryRowContext(ctx, `SELECT `+shareColumns+shareFrom+`
		WHERE s.id = ? AND s.game_id = ? AND s.created_by_user_id = ? AND g.status <> 'deleting'`, shareID, gameID, userID))
}

func (repository *Repository) ListShares(ctx context.Context, userID, gameID string) ([]Share, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT `+shareColumns+shareFrom+`
		WHERE s.game_id = ? AND s.created_by_user_id = ? AND g.status <> 'deleting'
		ORDER BY s.created_at DESC, s.id DESC LIMIT 100`, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer rows.Close()
	shares := make([]Share, 0)
	for rows.Next() {
		share, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate share links: %w", err)
	}
	if len(shares) == 0 {
		var count int
		if err := repository.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM games WHERE id = ? AND user_id = ? AND status <> 'deleting'`, gameID, userID).Scan(&count); err != nil {
			return nil, fmt.Errorf("check game for empty shares: %w", err)
		}
		if count == 0 {
			return nil, ErrNotFound
		}
	}
	return shares, nil
}

func (repository *Repository) RevokeShare(ctx context.Context, userID, gameID, shareID string, now time.Time) (Share, error) {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE share_links s
		JOIN games g ON g.id = s.game_id
		SET s.revoked_at = COALESCE(s.revoked_at, ?), s.revoke_reason = COALESCE(s.revoke_reason, 'creator_stopped')
		WHERE s.id = ? AND s.game_id = ? AND s.created_by_user_id = ? AND g.status <> 'deleting'`,
		now.UTC(), shareID, gameID, userID)
	if err != nil {
		return Share{}, fmt.Errorf("revoke share link: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		if _, err := repository.GetShare(ctx, userID, gameID, shareID); err != nil {
			return Share{}, err
		}
	}
	return repository.GetShare(ctx, userID, gameID, shareID)
}

func (repository *Repository) FindPublicShare(ctx context.Context, publicID string) (Share, error) {
	return scanShare(repository.db.QueryRowContext(ctx, `SELECT `+shareColumns+shareFrom+`
		WHERE s.public_id = ? AND g.status <> 'deleting' AND u.status = 'active'`, publicID))
}

func (repository *Repository) CreatePlaySession(ctx context.Context, sessionID, publicID string, secretHash, tokenHash []byte, expiresAt, now time.Time) (PlaySession, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return PlaySession{}, fmt.Errorf("begin play session: %w", err)
	}
	defer tx.Rollback()
	var shareID string
	var storedHash []byte
	var shareExpires time.Time
	var revokedAt sql.NullTime
	var gameStatus, versionStatus, userStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT s.id, s.secret_hash, s.expires_at, s.revoked_at, g.status, v.status, u.status
		FROM share_links s
		JOIN games g ON g.id = s.game_id
		JOIN game_versions v ON v.id = s.game_version_id
		JOIN users u ON u.id = s.created_by_user_id
		WHERE s.public_id = ? FOR UPDATE`, publicID,
	).Scan(&shareID, &storedHash, &shareExpires, &revokedAt, &gameStatus, &versionStatus, &userStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlaySession{}, ErrNotFound
		}
		return PlaySession{}, fmt.Errorf("lock share for play: %w", err)
	}
	if !equalHash(storedHash, secretHash) {
		return PlaySession{}, ErrNotFound
	}
	if revokedAt.Valid || !now.Before(shareExpires) || gameStatus == "deleting" || versionStatus != "ready" || userStatus != "active" {
		return PlaySession{}, ErrShareEnded
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO play_sessions (id, share_link_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, sessionID, shareID, tokenHash, expiresAt.UTC(), now.UTC()); err != nil {
		return PlaySession{}, fmt.Errorf("insert play session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PlaySession{}, fmt.Errorf("commit play session: %w", err)
	}
	return repository.FindPlaySession(ctx, tokenHash, now)
}

func (repository *Repository) FindPlaySession(ctx context.Context, tokenHash []byte, now time.Time) (PlaySession, error) {
	var session PlaySession
	err := repository.db.QueryRowContext(ctx, `
		SELECT p.id, p.share_link_id, p.expires_at,
		       g.id, v.id, g.title, v.template_id, v.template_version,
		       config.id, config.bucket, config.object_key, u.nickname,
		       s.expires_at, s.revoked_at, g.status, v.status
		FROM play_sessions p
		JOIN share_links s ON s.id = p.share_link_id
		JOIN games g ON g.id = s.game_id
		JOIN game_versions v ON v.id = s.game_version_id
		JOIN users u ON u.id = s.created_by_user_id
		JOIN assets config ON config.id = v.game_config_asset_id AND config.kind = 'game_artifact'
		WHERE p.token_hash = ? AND p.expires_at > ? AND s.expires_at > ? AND s.revoked_at IS NULL
		  AND g.status <> 'deleting' AND v.status = 'ready' AND u.status = 'active'`, tokenHash, now.UTC(), now.UTC()).Scan(
		&session.ID, &session.ShareLinkID, &session.ExpiresAt,
		&session.GameID, &session.GameVersionID, &session.GameTitle, &session.TemplateID, &session.TemplateVersion,
		&session.ConfigAssetID, &session.ConfigBucket, &session.ConfigObjectKey, &session.CreatorNickname,
		&session.ShareExpiresAt, &session.ShareRevokedAt, &session.GameStatus, &session.GameVersionStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PlaySession{}, ErrPlayExpired
	}
	if err != nil {
		return PlaySession{}, fmt.Errorf("find play session: %w", err)
	}
	_, _ = repository.db.ExecContext(ctx, `UPDATE play_sessions SET last_seen_at = ? WHERE id = ?`, now.UTC(), session.ID)
	return session, nil
}

func (repository *Repository) ListRenderAssets(ctx context.Context, session PlaySession) ([]RenderAsset, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT a.id, gva.role, a.mime_type, a.bucket, a.object_key
		FROM game_version_assets gva
		JOIN assets a ON a.id = gva.asset_id
		WHERE gva.game_version_id = ? AND gva.role = 'render' AND a.kind = 'game_render' AND a.internal_status IN ('ready', 'available')
		ORDER BY gva.sort_order, a.created_at, a.id`, session.GameVersionID)
	if err != nil {
		return nil, fmt.Errorf("list public render assets: %w", err)
	}
	defer rows.Close()
	assets := make([]RenderAsset, 0)
	for rows.Next() {
		var asset RenderAsset
		if err := rows.Scan(&asset.ID, &asset.Role, &asset.MIMEType, &asset.Bucket, &asset.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan public render asset: %w", err)
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func equalHash(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare(left, right) == 1
}
