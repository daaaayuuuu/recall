package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gamegen/backend/internal/invitations"
	"gamegen/backend/internal/platform/database"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrUserIDExists       = errors.New("user id already exists")
	ErrNotFound           = errors.New("record not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type User struct {
	ID            string
	LoginID       string
	PasswordHash  string
	Nickname      sql.NullString
	AvatarAssetID sql.NullString
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AvatarAsset struct {
	ID             string
	OwnerUserID    string
	Bucket         string
	ObjectKey      string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 []byte
	Width          int
	Height         int
	CreatedAt      time.Time
}

type StoredObject struct {
	Bucket    string
	ObjectKey string
}

type UserSession struct {
	ID            string
	User          User
	CSRFTokenHash []byte
	ExpiresAt     time.Time
}

type AdminSession struct {
	ID                    string
	Username              string
	CSRFTokenHash         []byte
	CredentialFingerprint []byte
	ExpiresAt             time.Time
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) CreateUser(ctx context.Context, user User) error {
	_, err := repository.db.ExecContext(ctx, `
		INSERT INTO users (id, login_id, password_hash, nickname, status, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), 'active', ?, ?)`,
		user.ID, user.LoginID, user.PasswordHash, user.Nickname.String, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return ErrUserIDExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (repository *Repository) CreateUserWithInvitation(ctx context.Context, user User, invitationHash []byte) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invited registration: %w", err)
	}
	defer tx.Rollback()

	var invitationID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM registration_invites
		WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL
		FOR UPDATE`, invitationHash).Scan(&invitationID); errors.Is(err, sql.ErrNoRows) {
		return invitations.ErrInvalidOrUsed
	} else if err != nil {
		return fmt.Errorf("lock registration invitation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, login_id, password_hash, nickname, status, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), 'active', ?, ?)`,
		user.ID, user.LoginID, user.PasswordHash, user.Nickname.String, user.CreatedAt, user.UpdatedAt,
	); err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return ErrUserIDExists
		}
		return fmt.Errorf("insert invited user: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE registration_invites
		SET used_by_user_id = ?, used_at = ?
		WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL`,
		user.ID, user.CreatedAt.UTC(), invitationID,
	)
	if err != nil {
		return fmt.Errorf("consume registration invitation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return invitations.ErrInvalidOrUsed
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invited registration: %w", err)
	}
	return nil
}

func (repository *Repository) FindUserByLoginID(ctx context.Context, loginID string) (User, error) {
	return scanUser(repository.db.QueryRowContext(ctx, `
		SELECT id, login_id, password_hash, nickname, avatar_asset_id, status, created_at, updated_at
		FROM users
		WHERE login_id = ?`, loginID))
}

func (repository *Repository) FindUserByID(ctx context.Context, id string) (User, error) {
	return scanUser(repository.db.QueryRowContext(ctx, `
		SELECT id, login_id, password_hash, nickname, avatar_asset_id, status, created_at, updated_at
		FROM users
		WHERE id = ?`, id))
}

func scanUser(row *sql.Row) (User, error) {
	return scanUserFrom(row)
}

type rowScanner interface {
	Scan(...any) error
}

func scanUserFrom(scanner rowScanner) (User, error) {
	var user User
	err := scanner.Scan(
		&user.ID, &user.LoginID, &user.PasswordHash, &user.Nickname, &user.AvatarAssetID,
		&user.Status, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("scan user: %w", err)
	}
	return user, nil
}

func (repository *Repository) CreateUserSession(ctx context.Context, id, userID string, tokenHash, csrfHash []byte, expiresAt, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user login: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE users SET last_login_at = ? WHERE id = ? AND status = 'active'`, now.UTC(), userID)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrInvalidCredentials
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, token_hash, csrf_token_hash, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, userID, tokenHash, csrfHash, expiresAt.UTC(), now.UTC()); err != nil {
		return fmt.Errorf("insert user session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user login: %w", err)
	}
	return nil
}

func (repository *Repository) GetUserSession(ctx context.Context, tokenHash []byte, now time.Time) (UserSession, error) {
	var session UserSession
	err := repository.db.QueryRowContext(ctx, `
		SELECT s.id, s.csrf_token_hash, s.expires_at,
		       u.id, u.login_id, u.password_hash, u.nickname, u.avatar_asset_id, u.status,
		       u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.status = 'active'`,
		tokenHash, now.UTC(),
	).Scan(
		&session.ID, &session.CSRFTokenHash, &session.ExpiresAt,
		&session.User.ID, &session.User.LoginID, &session.User.PasswordHash, &session.User.Nickname,
		&session.User.AvatarAssetID, &session.User.Status,
		&session.User.CreatedAt, &session.User.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UserSession{}, ErrNotFound
	}
	if err != nil {
		return UserSession{}, fmt.Errorf("get user session: %w", err)
	}
	return session, nil
}

func (repository *Repository) RotateUserCSRF(ctx context.Context, sessionID string, csrfHash []byte, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE user_sessions
		SET csrf_token_hash = ?, last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, csrfHash, now.UTC(), sessionID, now.UTC())
	if err != nil {
		return fmt.Errorf("rotate user csrf token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *Repository) RevokeUserSession(ctx context.Context, sessionID string, now time.Time) error {
	_, err := repository.db.ExecContext(ctx, `
		UPDATE user_sessions SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL`, now.UTC(), sessionID)
	if err != nil {
		return fmt.Errorf("revoke user session: %w", err)
	}
	return nil
}

func (repository *Repository) UpdateNickname(ctx context.Context, userID string, nickname sql.NullString) (User, error) {
	if _, err := repository.db.ExecContext(ctx, `
		UPDATE users SET nickname = ? WHERE id = ? AND status = 'active'`, nickname, userID); err != nil {
		return User{}, fmt.Errorf("update nickname: %w", err)
	}
	return repository.FindUserByID(ctx, userID)
}

func (repository *Repository) GetAvatar(ctx context.Context, userID string) (AvatarAsset, error) {
	var asset AvatarAsset
	err := repository.db.QueryRowContext(ctx, `
		SELECT a.id, a.owner_user_id, a.bucket, a.object_key, a.mime_type, a.size_bytes,
		       a.checksum_sha256, a.width, a.height, a.created_at
		FROM users u
		JOIN assets a ON a.id = u.avatar_asset_id
		WHERE u.id = ? AND u.status = 'active' AND a.owner_user_id = u.id AND a.kind = 'avatar'`, userID).Scan(
		&asset.ID, &asset.OwnerUserID, &asset.Bucket, &asset.ObjectKey, &asset.MIMEType,
		&asset.SizeBytes, &asset.ChecksumSHA256, &asset.Width, &asset.Height, &asset.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AvatarAsset{}, ErrNotFound
	}
	if err != nil {
		return AvatarAsset{}, fmt.Errorf("select avatar: %w", err)
	}
	return asset, nil
}

func (repository *Repository) ReplaceAvatar(ctx context.Context, userID string, asset AvatarAsset) (User, *StoredObject, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, nil, fmt.Errorf("begin avatar replacement: %w", err)
	}
	defer tx.Rollback()

	var previousID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT avatar_asset_id FROM users
		WHERE id = ? AND status = 'active' FOR UPDATE`, userID).Scan(&previousID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, nil, ErrNotFound
	}
	if err != nil {
		return User{}, nil, fmt.Errorf("lock avatar owner: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO assets
		(id, owner_user_id, kind, bucket, object_key, mime_type, size_bytes, checksum_sha256,
		 width, height, internal_status, created_at)
		VALUES (?, ?, 'avatar', ?, ?, ?, ?, ?, ?, ?, 'ready', ?)`,
		asset.ID, userID, asset.Bucket, asset.ObjectKey, asset.MIMEType, asset.SizeBytes,
		asset.ChecksumSHA256, asset.Width, asset.Height, asset.CreatedAt.UTC(),
	); err != nil {
		return User{}, nil, fmt.Errorf("insert avatar asset: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET avatar_asset_id = ? WHERE id = ?`, asset.ID, userID); err != nil {
		return User{}, nil, fmt.Errorf("select user avatar: %w", err)
	}

	var previous *StoredObject
	if previousID.Valid {
		previous = &StoredObject{}
		if err := tx.QueryRowContext(ctx, `
			SELECT bucket, object_key FROM assets
			WHERE id = ? AND owner_user_id = ? AND kind = 'avatar'`, previousID.String, userID).Scan(
			&previous.Bucket, &previous.ObjectKey,
		); err != nil {
			return User{}, nil, fmt.Errorf("select previous avatar object: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, previousID.String); err != nil {
			return User{}, nil, fmt.Errorf("delete previous avatar asset: %w", err)
		}
	}
	user, err := scanUserFrom(tx.QueryRowContext(ctx, `
		SELECT id, login_id, password_hash, nickname, avatar_asset_id, status, created_at, updated_at
		FROM users WHERE id = ?`, userID))
	if err != nil {
		return User{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, nil, fmt.Errorf("commit avatar replacement: %w", err)
	}
	return user, previous, nil
}

func (repository *Repository) RemoveAvatar(ctx context.Context, userID string) (User, *StoredObject, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, nil, fmt.Errorf("begin avatar removal: %w", err)
	}
	defer tx.Rollback()

	var avatarID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT avatar_asset_id FROM users
		WHERE id = ? AND status = 'active' FOR UPDATE`, userID).Scan(&avatarID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, nil, ErrNotFound
	}
	if err != nil {
		return User{}, nil, fmt.Errorf("lock avatar owner: %w", err)
	}
	if !avatarID.Valid {
		user, err := scanUserFrom(tx.QueryRowContext(ctx, `
			SELECT id, login_id, password_hash, nickname, avatar_asset_id, status, created_at, updated_at
			FROM users WHERE id = ?`, userID))
		if err != nil {
			return User{}, nil, err
		}
		if err := tx.Commit(); err != nil {
			return User{}, nil, fmt.Errorf("commit empty avatar removal: %w", err)
		}
		return user, nil, nil
	}

	object := &StoredObject{}
	if err := tx.QueryRowContext(ctx, `
		SELECT bucket, object_key FROM assets
		WHERE id = ? AND owner_user_id = ? AND kind = 'avatar'`, avatarID.String, userID).Scan(
		&object.Bucket, &object.ObjectKey,
	); err != nil {
		return User{}, nil, fmt.Errorf("select avatar object: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET avatar_asset_id = NULL WHERE id = ?`, userID); err != nil {
		return User{}, nil, fmt.Errorf("clear user avatar: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, avatarID.String); err != nil {
		return User{}, nil, fmt.Errorf("delete avatar asset: %w", err)
	}
	user, err := scanUserFrom(tx.QueryRowContext(ctx, `
		SELECT id, login_id, password_hash, nickname, avatar_asset_id, status, created_at, updated_at
		FROM users WHERE id = ?`, userID))
	if err != nil {
		return User{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, nil, fmt.Errorf("commit avatar removal: %w", err)
	}
	return user, object, nil
}

func (repository *Repository) ChangePassword(ctx context.Context, userID, passwordHash string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_sessions SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL`, now.UTC(), userID); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

func (repository *Repository) CreateAdminSession(ctx context.Context, session AdminSession, tokenHash []byte, now time.Time) error {
	_, err := repository.db.ExecContext(ctx, `
		INSERT INTO admin_sessions
		(id, admin_username, token_hash, csrf_token_hash, credential_fingerprint, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.Username, tokenHash, session.CSRFTokenHash,
		session.CredentialFingerprint, session.ExpiresAt.UTC(), now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert admin session: %w", err)
	}
	return nil
}

func (repository *Repository) GetAdminSession(ctx context.Context, tokenHash, credentialFingerprint []byte, now time.Time) (AdminSession, error) {
	var session AdminSession
	err := repository.db.QueryRowContext(ctx, `
		SELECT id, admin_username, csrf_token_hash, credential_fingerprint, expires_at
		FROM admin_sessions
		WHERE token_hash = ? AND credential_fingerprint = ?
		  AND revoked_at IS NULL AND expires_at > ?`,
		tokenHash, credentialFingerprint, now.UTC(),
	).Scan(&session.ID, &session.Username, &session.CSRFTokenHash, &session.CredentialFingerprint, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, fmt.Errorf("get admin session: %w", err)
	}
	return session, nil
}

func (repository *Repository) RotateAdminCSRF(ctx context.Context, sessionID string, csrfHash []byte, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE admin_sessions SET csrf_token_hash = ?, last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, csrfHash, now.UTC(), sessionID, now.UTC())
	if err != nil {
		return fmt.Errorf("rotate admin csrf token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *Repository) RevokeAdminSession(ctx context.Context, sessionID string, now time.Time) error {
	_, err := repository.db.ExecContext(ctx, `
		UPDATE admin_sessions SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL`, now.UTC(), sessionID)
	if err != nil {
		return fmt.Errorf("revoke admin session: %w", err)
	}
	return nil
}

func (repository *Repository) CreateAdminAuditLog(ctx context.Context, id, sessionID, username, action, requestID string, now time.Time) error {
	_, err := repository.db.ExecContext(ctx, `
		INSERT INTO admin_audit_logs
		(id, admin_session_id, actor_username, action, target_type, target_id, request_id, metadata, created_at)
		VALUES (?, ?, ?, ?, 'session', NULL, ?, JSON_OBJECT(), ?)`,
		id, sessionID, username, action, requestID, now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert admin audit log: %w", err)
	}
	return nil
}
