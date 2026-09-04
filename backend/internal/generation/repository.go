package generation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gamegen/backend/internal/gametemplates"
	"gamegen/backend/internal/platform/database"
)

var (
	ErrNotFound            = errors.New("generation run not found")
	ErrAssetsRequired      = errors.New("generation assets required")
	ErrVersionNotReady     = errors.New("version cannot be submitted")
	ErrActiveRun           = errors.New("an active generation run already exists")
	ErrIdempotencyConflict = errors.New("idempotency key was used for another request")
	ErrLeaseLost           = errors.New("generation lease lost")
	ErrCancellationWon     = errors.New("generation was cancelled before completion")
	ErrMaterialsIncomplete = errors.New("template materials are incomplete")
	ErrGenerationDisabled  = errors.New("generation is not enabled for template")
)

type Run struct {
	ID                string
	GameID            string
	GameVersionID     string
	AttemptNumber     int
	ExecutionCount    int
	TriggerType       string
	Status            string
	Stage             string
	Progress          int
	ErrorCode         sql.NullString
	AdminMessage      sql.NullString
	SanitizedDetails  []byte
	Retryable         bool
	TraceID           string
	LeaseOwner        sql.NullString
	LeaseExpiresAt    sql.NullTime
	HeartbeatAt       sql.NullTime
	CancelRequestedAt sql.NullTime
	NextAttemptAt     time.Time
	StartedAt         sql.NullTime
	CompletedAt       sql.NullTime
	CreatedAt         time.Time
	UpdatedAt         time.Time
	UserID            string
	GameTitle         string
	TemplateID        string
	TemplateVersion   string
}

type Artifact struct {
	ID             string
	OwnerUserID    string
	Bucket         string
	ObjectKey      string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 []byte
	CreatedAt      time.Time
	RenderAssets   []RenderAsset
}

type SourceAsset struct {
	Bucket         string
	ObjectKey      string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 []byte
	Width          int
	Height         int
	SlotKey        string
	SortOrder      int
}

type RenderAsset struct {
	ID             string
	OwnerUserID    string
	Bucket         string
	ObjectKey      string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 []byte
	Width          int
	Height         int
	SlotKey        string
	SortOrder      int
	CreatedAt      time.Time
}

type VersionInput struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
}

type Failure struct {
	Code             string
	AdminMessage     string
	SanitizedDetails map[string]any
	Retryable        bool
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) LoadVersionInput(ctx context.Context, gameID, versionID string) (VersionInput, error) {
	var input VersionInput
	err := repository.db.QueryRowContext(ctx, `
		SELECT input_payload_ciphertext, input_payload_nonce, encryption_key_version
		FROM game_versions WHERE id = ? AND game_id = ?`, versionID, gameID,
	).Scan(&input.Ciphertext, &input.Nonce, &input.KeyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return VersionInput{}, ErrNotFound
	}
	if err != nil {
		return VersionInput{}, fmt.Errorf("load generation version input: %w", err)
	}
	return input, nil
}

func (repository *Repository) LoadSourceAssets(ctx context.Context, userID, gameID, versionID string) ([]SourceAsset, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT a.bucket, a.object_key, a.mime_type, a.size_bytes, a.checksum_sha256,
		       COALESCE(a.width, 0), COALESCE(a.height, 0), COALESCE(gva.slot_key, ''), gva.sort_order
		FROM game_version_assets gva
		JOIN assets a ON a.id = gva.asset_id
		JOIN game_versions v ON v.id = gva.game_version_id
		JOIN games g ON g.id = v.game_id
		WHERE gva.game_version_id = ? AND v.game_id = ? AND g.user_id = ?
		  AND gva.role IN ('source', 'cover') AND a.kind IN ('game_source', 'game_cover')
		  AND a.internal_status IN ('ready', 'available')
		ORDER BY CASE WHEN gva.role = 'cover' THEN 0 ELSE 1 END,
		         COALESCE(gva.slot_key, ''), gva.sort_order, a.created_at, a.id`,
		versionID, gameID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list generation source assets: %w", err)
	}
	defer rows.Close()

	assets := make([]SourceAsset, 0)
	for rows.Next() {
		var asset SourceAsset
		if err := rows.Scan(
			&asset.Bucket, &asset.ObjectKey, &asset.MIMEType, &asset.SizeBytes, &asset.ChecksumSHA256,
			&asset.Width, &asset.Height, &asset.SlotKey, &asset.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("scan generation source asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation source assets: %w", err)
	}
	return assets, nil
}

func (repository *Repository) Submit(ctx context.Context, runID, traceID, userID, gameID, versionID string, idempotencyHash []byte, now time.Time) (Run, bool, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, false, fmt.Errorf("begin generation submission: %w", err)
	}
	defer tx.Rollback()

	var gameStatus string
	var currentVersionID sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT status, current_version_id FROM games
		WHERE id = ? AND user_id = ? AND status <> 'deleting' FOR UPDATE`, gameID, userID).Scan(&gameStatus, &currentVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, false, ErrNotFound
		}
		return Run{}, false, fmt.Errorf("lock game for generation: %w", err)
	}
	if !currentVersionID.Valid || currentVersionID.String != versionID {
		return Run{}, false, ErrVersionNotReady
	}

	if len(idempotencyHash) > 0 {
		existing, err := scanRun(tx.QueryRowContext(ctx, runSelect+`
			WHERE r.game_id = ? AND r.idempotency_key_hash = ?`, gameID, idempotencyHash))
		if err == nil {
			if existing.GameVersionID != versionID {
				return Run{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Run{}, false, err
		}
	}

	var versionStatus, templateID, templateVersion string
	if err := tx.QueryRowContext(ctx, `
		SELECT status, template_id, template_version FROM game_versions WHERE id = ? AND game_id = ? FOR UPDATE`, versionID, gameID).Scan(&versionStatus, &templateID, &templateVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, false, ErrNotFound
		}
		return Run{}, false, fmt.Errorf("validate generation version: %w", err)
	}
	if definition, ok := gametemplates.Find(templateID, templateVersion); ok {
		rows, err := tx.QueryContext(ctx, `
			SELECT COALESCE(slot_key, ''), COUNT(*) FROM game_version_assets
			WHERE game_version_id = ? AND role IN ('source', 'cover')
			GROUP BY slot_key`, versionID)
		if err != nil {
			return Run{}, false, fmt.Errorf("count template material slots: %w", err)
		}
		counts := make(map[string]int)
		for rows.Next() {
			var key string
			var count int
			if err := rows.Scan(&key, &count); err != nil {
				rows.Close()
				return Run{}, false, fmt.Errorf("scan template material count: %w", err)
			}
			counts[key] = count
		}
		if err := rows.Close(); err != nil {
			return Run{}, false, fmt.Errorf("close template material counts: %w", err)
		}
		for _, scene := range definition.Scenes {
			for _, slot := range scene.AssetSlots {
				if counts[slot.Key] < slot.MinItems {
					return Run{}, false, ErrMaterialsIncomplete
				}
			}
		}
		if !definition.GenerationEnabled {
			return Run{}, false, ErrGenerationDisabled
		}
	} else {
		var assetCount int
		if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM game_version_assets
		WHERE game_version_id = ? AND role IN ('source', 'cover')`, versionID).Scan(&assetCount); err != nil {
			return Run{}, false, fmt.Errorf("count generation assets: %w", err)
		}
		if assetCount == 0 {
			return Run{}, false, ErrAssetsRequired
		}
	}
	if versionStatus != "draft" && versionStatus != "failed" {
		return Run{}, false, ErrVersionNotReady
	}

	var activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM game_generation_runs
		WHERE game_version_id = ? AND status IN ('queued', 'running')`, versionID).Scan(&activeCount); err != nil {
		return Run{}, false, fmt.Errorf("check active generation run: %w", err)
	}
	if activeCount > 0 || gameStatus == "queued" || gameStatus == "generating" {
		return Run{}, false, ErrActiveRun
	}

	var attemptNumber int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(attempt_number), 0) + 1
		FROM game_generation_runs WHERE game_version_id = ?`, versionID).Scan(&attemptNumber); err != nil {
		return Run{}, false, fmt.Errorf("select generation attempt: %w", err)
	}
	triggerType := "initial"
	if attemptNumber > 1 {
		triggerType = "user_retry"
	}
	var key any
	if len(idempotencyHash) > 0 {
		key = idempotencyHash
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO game_generation_runs
		(id, game_id, game_version_id, attempt_number, execution_count, trigger_type,
		 idempotency_key_hash, status, stage, progress, retryable, trace_id, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?, 'queued', 'queued', 0, FALSE, ?, ?, ?, ?)`,
		runID, gameID, versionID, attemptNumber, triggerType, key, traceID, now.UTC(), now.UTC(), now.UTC())
	if err != nil {
		return Run{}, false, fmt.Errorf("insert generation run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE games SET status = 'queued', current_generation_run_id = ?, updated_at = ?
		WHERE id = ?`, runID, now.UTC(), gameID); err != nil {
		return Run{}, false, fmt.Errorf("queue game: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_versions SET status = 'queued', updated_at = ? WHERE id = ?`, now.UTC(), versionID); err != nil {
		return Run{}, false, fmt.Errorf("queue game version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, fmt.Errorf("commit generation submission: %w", err)
	}
	created, err := repository.Get(ctx, userID, gameID, runID)
	return created, false, err
}

const runColumns = `
	r.id, r.game_id, r.game_version_id, r.attempt_number, r.execution_count, r.trigger_type,
	r.status, r.stage, r.progress, r.error_code, r.admin_message, r.sanitized_details,
	r.retryable, r.trace_id, r.lease_owner, r.lease_expires_at, r.heartbeat_at,
	r.cancel_requested_at, r.next_attempt_at, r.started_at, r.completed_at, r.created_at, r.updated_at`

const runSelect = `SELECT ` + runColumns + ` FROM game_generation_runs r `

type scanner interface {
	Scan(...any) error
}

func scanRun(row scanner) (Run, error) {
	var run Run
	err := row.Scan(
		&run.ID, &run.GameID, &run.GameVersionID, &run.AttemptNumber, &run.ExecutionCount, &run.TriggerType,
		&run.Status, &run.Stage, &run.Progress, &run.ErrorCode, &run.AdminMessage, &run.SanitizedDetails,
		&run.Retryable, &run.TraceID, &run.LeaseOwner, &run.LeaseExpiresAt, &run.HeartbeatAt,
		&run.CancelRequestedAt, &run.NextAttemptAt, &run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("scan generation run: %w", err)
	}
	return run, nil
}

func (repository *Repository) Get(ctx context.Context, userID, gameID, runID string) (Run, error) {
	return scanRun(repository.db.QueryRowContext(ctx, runSelect+`
		JOIN games g ON g.id = r.game_id
		WHERE r.id = ? AND r.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'`, runID, gameID, userID))
}

func (repository *Repository) List(ctx context.Context, userID, gameID string) ([]Run, error) {
	rows, err := repository.db.QueryContext(ctx, runSelect+`
		JOIN games g ON g.id = r.game_id
		WHERE r.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'
		ORDER BY r.created_at DESC, r.id DESC LIMIT 100`, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("list generation runs: %w", err)
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate generation runs: %w", err)
	}
	return runs, nil
}

func (repository *Repository) RequestCancel(ctx context.Context, userID, gameID, runID string, now time.Time) (Run, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, fmt.Errorf("begin generation cancellation: %w", err)
	}
	defer tx.Rollback()
	run, err := scanRun(tx.QueryRowContext(ctx, runSelect+`
		JOIN games g ON g.id = r.game_id
		WHERE r.id = ? AND r.game_id = ? AND g.user_id = ? AND g.status <> 'deleting' FOR UPDATE`, runID, gameID, userID))
	if err != nil {
		return Run{}, err
	}
	switch run.Status {
	case "queued":
		if _, err := tx.ExecContext(ctx, `
			UPDATE game_generation_runs
			SET status = 'cancelled', stage = 'completed', cancel_requested_at = ?, completed_at = ?, updated_at = ?
			WHERE id = ?`, now.UTC(), now.UTC(), now.UTC(), runID); err != nil {
			return Run{}, fmt.Errorf("cancel queued generation: %w", err)
		}
		if err := resetGameAfterCancellation(ctx, tx, run, now); err != nil {
			return Run{}, err
		}
		run.Status, run.Stage = "cancelled", "completed"
		run.CancelRequestedAt, run.CompletedAt = sql.NullTime{Time: now.UTC(), Valid: true}, sql.NullTime{Time: now.UTC(), Valid: true}
	case "running":
		if _, err := tx.ExecContext(ctx, `
			UPDATE game_generation_runs SET cancel_requested_at = COALESCE(cancel_requested_at, ?), updated_at = ?
			WHERE id = ?`, now.UTC(), now.UTC(), runID); err != nil {
			return Run{}, fmt.Errorf("request running generation cancellation: %w", err)
		}
		run.CancelRequestedAt = sql.NullTime{Time: now.UTC(), Valid: true}
	}
	if err := tx.Commit(); err != nil {
		return Run{}, fmt.Errorf("commit generation cancellation: %w", err)
	}
	return run, nil
}

func (repository *Repository) Claim(ctx context.Context, workerID string, maxExecutions int, leaseDuration time.Duration, now time.Time) (Run, bool, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, false, fmt.Errorf("begin generation claim: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+runColumns+`, g.user_id, g.title, v.template_id, v.template_version
		FROM game_generation_runs r
		JOIN games g ON g.id = r.game_id
		JOIN game_versions v ON v.id = r.game_version_id
		WHERE (r.status = 'queued' AND r.next_attempt_at <= ?)
		   OR (r.status = 'running' AND r.lease_expires_at <= ?)
		ORDER BY r.next_attempt_at, r.created_at LIMIT 1 FOR UPDATE SKIP LOCKED`, now.UTC(), now.UTC())
	var run Run
	err = row.Scan(
		&run.ID, &run.GameID, &run.GameVersionID, &run.AttemptNumber, &run.ExecutionCount, &run.TriggerType,
		&run.Status, &run.Stage, &run.Progress, &run.ErrorCode, &run.AdminMessage, &run.SanitizedDetails,
		&run.Retryable, &run.TraceID, &run.LeaseOwner, &run.LeaseExpiresAt, &run.HeartbeatAt,
		&run.CancelRequestedAt, &run.NextAttemptAt, &run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
		&run.UserID, &run.GameTitle, &run.TemplateID, &run.TemplateVersion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, fmt.Errorf("select generation queue: %w", err)
	}

	if run.CancelRequestedAt.Valid {
		if err := cancelClaimedRun(ctx, tx, run, now); err != nil {
			return Run{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return Run{}, false, fmt.Errorf("commit claimed cancellation: %w", err)
		}
		run.Status, run.Stage = "cancelled", "completed"
		return run, true, nil
	}
	if run.Status == "running" && run.ExecutionCount >= maxExecutions {
		failure := Failure{Code: "TASK_LEASE_EXHAUSTED", AdminMessage: "任务租约多次过期，已停止自动恢复", SanitizedDetails: map[string]any{"executionCount": run.ExecutionCount}, Retryable: true}
		if err := failClaimedRun(ctx, tx, run, failure, now); err != nil {
			return Run{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return Run{}, false, fmt.Errorf("commit exhausted generation: %w", err)
		}
		run.Status, run.Stage, run.Progress, run.ErrorCode, run.Retryable = "failed", "completed", 100, sql.NullString{String: failure.Code, Valid: true}, true
		return run, true, nil
	}

	leaseExpires := now.Add(leaseDuration).UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_generation_runs
		SET status = 'running', execution_count = execution_count + 1, lease_owner = ?,
		    lease_expires_at = ?, heartbeat_at = ?, started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = ?`, workerID, leaseExpires, now.UTC(), now.UTC(), now.UTC(), run.ID); err != nil {
		return Run{}, false, fmt.Errorf("claim generation run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET status = 'generating', updated_at = ? WHERE id = ?`, now.UTC(), run.GameID); err != nil {
		return Run{}, false, fmt.Errorf("mark game generating: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_versions SET status = 'generating', updated_at = ? WHERE id = ?`, now.UTC(), run.GameVersionID); err != nil {
		return Run{}, false, fmt.Errorf("mark version generating: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, fmt.Errorf("commit generation claim: %w", err)
	}
	run.Status = "running"
	run.ExecutionCount++
	run.LeaseOwner = sql.NullString{String: workerID, Valid: true}
	run.LeaseExpiresAt = sql.NullTime{Time: leaseExpires, Valid: true}
	return run, true, nil
}

func (repository *Repository) UpdateProgress(ctx context.Context, runID, workerID, stage string, progress int, leaseDuration time.Duration, now time.Time) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE game_generation_runs
		SET stage = ?, progress = ?, heartbeat_at = ?, lease_expires_at = ?, updated_at = ?
		WHERE id = ? AND status = 'running' AND lease_owner = ?`,
		stage, progress, now.UTC(), now.Add(leaseDuration).UTC(), now.UTC(), runID, workerID)
	if err != nil {
		return false, fmt.Errorf("update generation progress: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, ErrLeaseLost
	}
	var cancelRequested sql.NullTime
	if err := repository.db.QueryRowContext(ctx, `SELECT cancel_requested_at FROM game_generation_runs WHERE id = ?`, runID).Scan(&cancelRequested); err != nil {
		return false, fmt.Errorf("check generation cancellation: %w", err)
	}
	return cancelRequested.Valid, nil
}

func (repository *Repository) CompleteCancelled(ctx context.Context, run Run, workerID string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin running generation cancellation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE game_generation_runs
		SET status = 'cancelled', stage = 'completed', lease_owner = NULL, lease_expires_at = NULL,
		    completed_at = ?, updated_at = ?
		WHERE id = ? AND status = 'running' AND lease_owner = ?`, now.UTC(), now.UTC(), run.ID, workerID)
	if err != nil {
		return fmt.Errorf("complete generation cancellation: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrLeaseLost
	}
	if err := resetGameAfterCancellation(ctx, tx, run, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *Repository) Fail(ctx context.Context, run Run, workerID string, failure Failure, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin generation failure: %w", err)
	}
	defer tx.Rollback()
	if err := failOwnedRun(ctx, tx, run, workerID, failure, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *Repository) CompleteSuccess(ctx context.Context, run Run, workerID string, artifact Artifact, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin generation success: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE game_generation_runs
		SET status = 'succeeded', stage = 'completed', progress = 100, error_code = NULL,
		    admin_message = NULL, sanitized_details = NULL, retryable = FALSE,
		    lease_owner = NULL, lease_expires_at = NULL, completed_at = ?, updated_at = ?
		WHERE id = ? AND status = 'running' AND lease_owner = ? AND cancel_requested_at IS NULL`, now.UTC(), now.UTC(), run.ID, workerID)
	if err != nil {
		return fmt.Errorf("complete generation run: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		var status string
		var owner sql.NullString
		var cancelRequested sql.NullTime
		if err := tx.QueryRowContext(ctx, `
			SELECT status, lease_owner, cancel_requested_at FROM game_generation_runs WHERE id = ? FOR UPDATE`, run.ID,
		).Scan(&status, &owner, &cancelRequested); err != nil {
			return fmt.Errorf("inspect generation completion race: %w", err)
		}
		if status == "running" && owner.Valid && owner.String == workerID && cancelRequested.Valid {
			if err := cancelClaimedRun(ctx, tx, run, now); err != nil {
				return err
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit generation cancellation race: %w", err)
			}
			return ErrCancellationWon
		}
		return ErrLeaseLost
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO assets
		(id, owner_user_id, kind, bucket, object_key, mime_type, size_bytes, checksum_sha256, internal_status, created_at)
		VALUES (?, ?, 'game_artifact', ?, ?, ?, ?, ?, 'available', ?)`,
		artifact.ID, artifact.OwnerUserID, artifact.Bucket, artifact.ObjectKey, artifact.MIMEType,
		artifact.SizeBytes, artifact.ChecksumSHA256, artifact.CreatedAt.UTC()); err != nil {
		return fmt.Errorf("insert generated artifact: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO game_version_assets (game_version_id, asset_id, role, sort_order, created_at)
		VALUES (?, ?, 'artifact', 0, ?)`, run.GameVersionID, artifact.ID, now.UTC()); err != nil {
		return fmt.Errorf("link generated artifact: %w", err)
	}
	for _, render := range artifact.RenderAssets {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO assets
			(id, owner_user_id, kind, bucket, object_key, mime_type, size_bytes, checksum_sha256,
			 width, height, internal_status, created_at)
			VALUES (?, ?, 'game_render', ?, ?, ?, ?, ?, ?, ?, 'available', ?)`,
			render.ID, render.OwnerUserID, render.Bucket, render.ObjectKey, render.MIMEType,
			render.SizeBytes, render.ChecksumSHA256, render.Width, render.Height, render.CreatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert generated render asset: %w", err)
		}
		var slotKey any
		if render.SlotKey != "" {
			slotKey = render.SlotKey
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO game_version_assets (game_version_id, asset_id, role, slot_key, sort_order, created_at)
			VALUES (?, ?, 'render', ?, ?, ?)`,
			run.GameVersionID, render.ID, slotKey, render.SortOrder, render.CreatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("link generated render asset: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_versions SET status = 'ready', game_config_asset_id = ?, updated_at = ? WHERE id = ?`,
		artifact.ID, now.UTC(), run.GameVersionID); err != nil {
		return fmt.Errorf("complete generated version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE games SET status = 'ready', current_version_id = ?, current_generation_run_id = ?, updated_at = ? WHERE id = ?`,
		run.GameVersionID, run.ID, now.UTC(), run.GameID); err != nil {
		return fmt.Errorf("complete generated game: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit generation success: %w", err)
	}
	return nil
}

func resetGameAfterCancellation(ctx context.Context, tx *sql.Tx, run Run, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE game_versions SET status = 'draft', updated_at = ? WHERE id = ?`, now.UTC(), run.GameVersionID); err != nil {
		return fmt.Errorf("reset cancelled version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET status = 'draft', updated_at = ? WHERE id = ? AND current_generation_run_id = ?`, now.UTC(), run.GameID, run.ID); err != nil {
		return fmt.Errorf("reset cancelled game: %w", err)
	}
	return nil
}

func cancelClaimedRun(ctx context.Context, tx *sql.Tx, run Run, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_generation_runs SET status = 'cancelled', stage = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		now.UTC(), now.UTC(), run.ID); err != nil {
		return fmt.Errorf("cancel claimed generation: %w", err)
	}
	return resetGameAfterCancellation(ctx, tx, run, now)
}

func failClaimedRun(ctx context.Context, tx *sql.Tx, run Run, failure Failure, now time.Time) error {
	return failRun(ctx, tx, run, "", failure, now, false)
}

func failOwnedRun(ctx context.Context, tx *sql.Tx, run Run, workerID string, failure Failure, now time.Time) error {
	return failRun(ctx, tx, run, workerID, failure, now, true)
}

func failRun(ctx context.Context, tx *sql.Tx, run Run, workerID string, failure Failure, now time.Time, requireOwner bool) error {
	details, err := json.Marshal(failure.SanitizedDetails)
	if err != nil {
		return fmt.Errorf("encode sanitized generation failure: %w", err)
	}
	query := `UPDATE game_generation_runs
		SET status = 'failed', stage = 'completed', progress = 100, error_code = ?, admin_message = ?,
		    sanitized_details = ?, retryable = ?, lease_owner = NULL, lease_expires_at = NULL,
		    completed_at = ?, updated_at = ? WHERE id = ? AND status = 'running'`
	args := []any{failure.Code, failure.AdminMessage, details, failure.Retryable, now.UTC(), now.UTC(), run.ID}
	if requireOwner {
		query += ` AND lease_owner = ?`
		args = append(args, workerID)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("fail generation run: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrLeaseLost
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_versions SET status = 'failed', updated_at = ? WHERE id = ?`, now.UTC(), run.GameVersionID); err != nil {
		return fmt.Errorf("fail generated version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET status = 'failed', updated_at = ? WHERE id = ?`, now.UTC(), run.GameID); err != nil {
		return fmt.Errorf("fail generated game: %w", err)
	}
	return nil
}

func (repository *Repository) ListAdmin(ctx context.Context, status string) ([]Run, error) {
	query := runSelect
	args := make([]any, 0, 1)
	if status != "" {
		query += ` WHERE r.status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY r.created_at DESC, r.id DESC LIMIT 100`
	rows, err := repository.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin generation runs: %w", err)
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (repository *Repository) GetAdmin(ctx context.Context, runID string) (Run, error) {
	return scanRun(repository.db.QueryRowContext(ctx, runSelect+` WHERE r.id = ?`, runID))
}
