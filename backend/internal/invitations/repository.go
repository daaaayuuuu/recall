package invitations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gamegen/backend/internal/platform/database"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrCodeCollision = errors.New("invitation code collision")
	ErrNotFound      = errors.New("invitation not found")
	ErrNotRevocable  = errors.New("invitation cannot be revoked")
)

type Invitation struct {
	ID              string
	CodeSuffix      string
	CreatedByAdmin  string
	UsedByCreatorID sql.NullString
	UsedByLoginID   sql.NullString
	UsedAt          sql.NullTime
	RevokedAt       sql.NullTime
	CreatedAt       time.Time
}

func (invitation Invitation) Status() string {
	if invitation.UsedAt.Valid {
		return "used"
	}
	if invitation.RevokedAt.Valid {
		return "revoked"
	}
	return "unused"
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) Create(
	ctx context.Context,
	invitation Invitation,
	codeHash []byte,
	adminSessionID, requestID, auditID string,
) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invitation creation: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO registration_invites
		(id, code_hash, code_suffix, created_by_admin, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		invitation.ID, codeHash, invitation.CodeSuffix, invitation.CreatedByAdmin, invitation.CreatedAt.UTC(),
	); err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return ErrCodeCollision
		}
		return fmt.Errorf("insert invitation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_audit_logs
		(id, admin_session_id, actor_username, action, target_type, target_id, request_id, metadata, created_at)
		VALUES (?, ?, ?, 'invitation.created', 'invitation', ?, ?, JSON_OBJECT('codeSuffix', ?), ?)`,
		auditID, adminSessionID, invitation.CreatedByAdmin, invitation.ID, requestID, invitation.CodeSuffix, invitation.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("audit invitation creation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invitation creation: %w", err)
	}
	return nil
}

func (repository *Repository) List(ctx context.Context, limit int) ([]Invitation, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT i.id, i.code_suffix, i.created_by_admin, i.used_by_user_id, u.login_id,
		       i.used_at, i.revoked_at, i.created_at
		FROM registration_invites i
		LEFT JOIN users u ON u.id = i.used_by_user_id
		ORDER BY i.created_at DESC, i.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	items := make([]Invitation, 0)
	for rows.Next() {
		var invitation Invitation
		if err := rows.Scan(
			&invitation.ID, &invitation.CodeSuffix, &invitation.CreatedByAdmin,
			&invitation.UsedByCreatorID, &invitation.UsedByLoginID,
			&invitation.UsedAt, &invitation.RevokedAt, &invitation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		items = append(items, invitation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invitations: %w", err)
	}
	return items, nil
}

func (repository *Repository) Revoke(
	ctx context.Context,
	id, adminSessionID, adminUsername, requestID, auditID string,
	now time.Time,
) (Invitation, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, fmt.Errorf("begin invitation revocation: %w", err)
	}
	defer tx.Rollback()

	invitation, err := scanInvitation(tx.QueryRowContext(ctx, `
		SELECT i.id, i.code_suffix, i.created_by_admin, i.used_by_user_id, u.login_id,
		       i.used_at, i.revoked_at, i.created_at
		FROM registration_invites i
		LEFT JOIN users u ON u.id = i.used_by_user_id
		WHERE i.id = ? FOR UPDATE`, id))
	if err != nil {
		return Invitation{}, err
	}
	if invitation.UsedAt.Valid || invitation.RevokedAt.Valid {
		return Invitation{}, ErrNotRevocable
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE registration_invites SET revoked_at = ?
		WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL`, now.UTC(), id); err != nil {
		return Invitation{}, fmt.Errorf("revoke invitation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_audit_logs
		(id, admin_session_id, actor_username, action, target_type, target_id, request_id, metadata, created_at)
		VALUES (?, ?, ?, 'invitation.revoked', 'invitation', ?, ?, JSON_OBJECT('codeSuffix', ?), ?)`,
		auditID, adminSessionID, adminUsername, invitation.ID, requestID, invitation.CodeSuffix, now.UTC(),
	); err != nil {
		return Invitation{}, fmt.Errorf("audit invitation revocation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, fmt.Errorf("commit invitation revocation: %w", err)
	}
	invitation.RevokedAt = sql.NullTime{Time: now.UTC(), Valid: true}
	return invitation, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanInvitation(scanner rowScanner) (Invitation, error) {
	var invitation Invitation
	if err := scanner.Scan(
		&invitation.ID, &invitation.CodeSuffix, &invitation.CreatedByAdmin,
		&invitation.UsedByCreatorID, &invitation.UsedByLoginID,
		&invitation.UsedAt, &invitation.RevokedAt, &invitation.CreatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, ErrNotFound
	} else if err != nil {
		return Invitation{}, fmt.Errorf("scan invitation: %w", err)
	}
	return invitation, nil
}
