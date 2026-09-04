package games

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gamegen/backend/internal/platform/database"
)

var (
	ErrNotFound        = errors.New("game resource not found")
	ErrVersionNotDraft = errors.New("game version is not editable")
	ErrVersionNotReady = errors.New("game version is not ready for preview")
	ErrGameNotEditable = errors.New("game is not editable")
	ErrAssetSlotFull   = errors.New("asset slot is full")
	ErrAssetOrder      = errors.New("asset order does not match slot assets")
)

type Game struct {
	ID               string
	UserID           string
	Title            string
	Description      sql.NullString
	Status           string
	CoverAssetID     sql.NullString
	CoverBucket      sql.NullString
	CoverObjectKey   sql.NullString
	CurrentVersionID sql.NullString
	AssetCount       int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Version struct {
	ID                     string
	GameID                 string
	VersionNumber          int
	Status                 string
	InputSchemaVersion     int
	InputPayloadCiphertext []byte
	InputPayloadNonce      []byte
	EncryptionKeyVersion   int
	TemplateID             string
	TemplateVersion        string
	AssetCount             int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type Asset struct {
	ID             string
	GameVersionID  string
	Kind           string
	Role           string
	SlotKey        string
	Bucket         string
	ObjectKey      string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 []byte
	Width          int
	Height         int
	SortOrder      int
	CreatedAt      time.Time
}

type Preview struct {
	GameID          string
	GameTitle       string
	VersionID       string
	VersionNumber   int
	VersionStatus   string
	TemplateID      string
	TemplateVersion string
	ConfigAssetID   sql.NullString
	ConfigBucket    sql.NullString
	ConfigObjectKey sql.NullString
}

type PreviewAsset struct {
	ID        string
	MIMEType  string
	Bucket    string
	ObjectKey string
}

type EncryptedInput struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
}

type DeletionPayload struct {
	ObjectKeys []ObjectRef `json:"objectKeys"`
	AssetIDs   []string    `json:"assetIds"`
}

type ObjectRef struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type DeletionJob struct {
	ID      string
	GameID  string
	Payload DeletionPayload
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) CreateGame(ctx context.Context, gameID, versionID, userID, title string, description sql.NullString, input EncryptedInput, inputSchemaVersion int, templateID, templateVersion string, now time.Time) (Game, Version, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Game{}, Version{}, fmt.Errorf("begin game creation: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO games (id, user_id, title, description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'draft', ?, ?)`, gameID, userID, title, description, now.UTC(), now.UTC())
	if err != nil {
		return Game{}, Version{}, fmt.Errorf("insert game: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO game_versions
		(id, game_id, version_number, status, input_schema_version, input_payload_ciphertext,
		 input_payload_nonce, encryption_key_version, template_id, template_version, created_at, updated_at)
		VALUES (?, ?, 1, 'draft', ?, ?, ?, ?, ?, ?, ?, ?)`,
		versionID, gameID, inputSchemaVersion, input.Ciphertext, input.Nonce, input.KeyVersion,
		templateID, templateVersion, now.UTC(), now.UTC())
	if err != nil {
		return Game{}, Version{}, fmt.Errorf("insert initial game version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET current_version_id = ? WHERE id = ?`, versionID, gameID); err != nil {
		return Game{}, Version{}, fmt.Errorf("select current game version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Game{}, Version{}, fmt.Errorf("commit game creation: %w", err)
	}

	game, err := repository.GetGame(ctx, userID, gameID)
	if err != nil {
		return Game{}, Version{}, err
	}
	version, err := repository.GetVersion(ctx, userID, gameID, versionID)
	return game, version, err
}

func (repository *Repository) ListGames(ctx context.Context, userID string) ([]Game, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT g.id, g.user_id, g.title, g.description, g.status,
		       g.cover_asset_id, cover.bucket, cover.object_key, g.current_version_id,
		       g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM game_version_assets gva WHERE gva.game_version_id = g.current_version_id AND gva.role IN ('source', 'cover'))
		FROM games g
		LEFT JOIN assets cover ON cover.id = g.cover_asset_id
		WHERE g.user_id = ? AND g.status <> 'deleting'
		ORDER BY g.updated_at DESC, g.id DESC
		LIMIT 100`, userID)
	if err != nil {
		return nil, fmt.Errorf("list games: %w", err)
	}
	defer rows.Close()

	games := make([]Game, 0)
	for rows.Next() {
		game, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate games: %w", err)
	}
	return games, nil
}

func (repository *Repository) GetGame(ctx context.Context, userID, gameID string) (Game, error) {
	return scanGame(repository.db.QueryRowContext(ctx, `
		SELECT g.id, g.user_id, g.title, g.description, g.status,
		       g.cover_asset_id, cover.bucket, cover.object_key, g.current_version_id,
		       g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM game_version_assets gva WHERE gva.game_version_id = g.current_version_id AND gva.role IN ('source', 'cover'))
		FROM games g
		LEFT JOIN assets cover ON cover.id = g.cover_asset_id
		WHERE g.id = ? AND g.user_id = ? AND g.status <> 'deleting'`, gameID, userID))
}

type scanner interface {
	Scan(...any) error
}

func scanGame(row scanner) (Game, error) {
	var game Game
	err := row.Scan(
		&game.ID, &game.UserID, &game.Title, &game.Description, &game.Status,
		&game.CoverAssetID, &game.CoverBucket, &game.CoverObjectKey,
		&game.CurrentVersionID, &game.CreatedAt, &game.UpdatedAt, &game.AssetCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, fmt.Errorf("scan game: %w", err)
	}
	return game, nil
}

func (repository *Repository) UpdateGame(ctx context.Context, userID, gameID, title string, description sql.NullString, now time.Time) (Game, error) {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE games SET title = ?, description = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND status <> 'deleting'`, title, description, now.UTC(), gameID, userID)
	if err != nil {
		return Game{}, fmt.Errorf("update game: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return Game{}, ErrNotFound
	}
	return repository.GetGame(ctx, userID, gameID)
}

func (repository *Repository) CreateVersion(ctx context.Context, versionID, userID, gameID string, input EncryptedInput, inputSchemaVersion int, now time.Time) (Version, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, fmt.Errorf("begin version creation: %w", err)
	}
	defer tx.Rollback()

	var status string
	var currentVersionID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, current_version_id FROM games WHERE id = ? AND user_id = ? FOR UPDATE`, gameID, userID).Scan(&status, &currentVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("lock game for version: %w", err)
	}
	if status == "queued" || status == "generating" || status == "deleting" {
		return Version{}, ErrGameNotEditable
	}
	if !currentVersionID.Valid {
		return Version{}, ErrNotFound
	}
	var templateID, templateVersion string
	if err := tx.QueryRowContext(ctx, `SELECT template_id, template_version FROM game_versions WHERE id = ? AND game_id = ?`, currentVersionID.String, gameID).Scan(&templateID, &templateVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("select current version template: %w", err)
	}

	var versionNumber int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number), 0) + 1 FROM game_versions WHERE game_id = ?`, gameID).Scan(&versionNumber); err != nil {
		return Version{}, fmt.Errorf("select next version number: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO game_versions
		(id, game_id, version_number, status, input_schema_version, input_payload_ciphertext,
		 input_payload_nonce, encryption_key_version, template_id, template_version, created_at, updated_at)
		VALUES (?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?)`,
		versionID, gameID, versionNumber, inputSchemaVersion, input.Ciphertext, input.Nonce, input.KeyVersion,
		templateID, templateVersion, now.UTC(), now.UTC())
	if err != nil {
		return Version{}, fmt.Errorf("insert game version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO game_version_assets (game_version_id, asset_id, role, slot_key, sort_order, created_at)
		SELECT ?, asset_id, role, slot_key, sort_order, ?
		FROM game_version_assets
		WHERE game_version_id = ? AND role IN ('source', 'cover')`,
		versionID, now.UTC(), currentVersionID.String,
	); err != nil {
		return Version{}, fmt.Errorf("inherit current version assets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE games
		SET current_version_id = ?, current_generation_run_id = NULL, status = 'draft', updated_at = ?
		WHERE id = ?`, versionID, now.UTC(), gameID); err != nil {
		return Version{}, fmt.Errorf("update current game version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Version{}, fmt.Errorf("commit version creation: %w", err)
	}
	return repository.GetVersion(ctx, userID, gameID, versionID)
}

func (repository *Repository) ListVersions(ctx context.Context, userID, gameID string) ([]Version, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT v.id, v.game_id, v.version_number, v.status, v.input_schema_version,
		       v.input_payload_ciphertext, v.input_payload_nonce, v.encryption_key_version,
		       v.template_id, v.template_version, v.created_at, v.updated_at,
		       (SELECT COUNT(*) FROM game_version_assets gva WHERE gva.game_version_id = v.id AND gva.role IN ('source', 'cover'))
		FROM game_versions v
		JOIN games g ON g.id = v.game_id
		WHERE v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'
		ORDER BY v.version_number DESC`, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("list game versions: %w", err)
	}
	defer rows.Close()
	versions := make([]Version, 0)
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate game versions: %w", err)
	}
	if len(versions) == 0 {
		if _, err := repository.GetGame(ctx, userID, gameID); err != nil {
			return nil, err
		}
	}
	return versions, nil
}

func (repository *Repository) GetVersion(ctx context.Context, userID, gameID, versionID string) (Version, error) {
	return scanVersion(repository.db.QueryRowContext(ctx, `
		SELECT v.id, v.game_id, v.version_number, v.status, v.input_schema_version,
		       v.input_payload_ciphertext, v.input_payload_nonce, v.encryption_key_version,
		       v.template_id, v.template_version, v.created_at, v.updated_at,
		       (SELECT COUNT(*) FROM game_version_assets gva WHERE gva.game_version_id = v.id AND gva.role IN ('source', 'cover'))
		FROM game_versions v
		JOIN games g ON g.id = v.game_id
		WHERE v.id = ? AND v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'`,
		versionID, gameID, userID))
}

func (repository *Repository) GetPreview(ctx context.Context, userID, gameID, versionID string) (Preview, error) {
	var preview Preview
	err := repository.db.QueryRowContext(ctx, `
		SELECT g.id, g.title, v.id, v.version_number, v.status, v.template_id, v.template_version,
		       config.id, config.bucket, config.object_key
		FROM game_versions v
		JOIN games g ON g.id = v.game_id
		LEFT JOIN assets config ON config.id = v.game_config_asset_id
		  AND config.kind = 'game_artifact' AND config.internal_status IN ('ready', 'available')
		WHERE v.id = ? AND v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'`,
		versionID, gameID, userID,
	).Scan(
		&preview.GameID, &preview.GameTitle, &preview.VersionID, &preview.VersionNumber,
		&preview.VersionStatus, &preview.TemplateID, &preview.TemplateVersion,
		&preview.ConfigAssetID, &preview.ConfigBucket, &preview.ConfigObjectKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Preview{}, ErrNotFound
	}
	if err != nil {
		return Preview{}, fmt.Errorf("get game preview: %w", err)
	}
	if preview.VersionStatus != "ready" || !preview.ConfigAssetID.Valid || !preview.ConfigBucket.Valid || !preview.ConfigObjectKey.Valid {
		return Preview{}, ErrVersionNotReady
	}
	return preview, nil
}

func (repository *Repository) ListPreviewAssets(ctx context.Context, userID, gameID, versionID string) ([]PreviewAsset, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT a.id, a.mime_type, a.bucket, a.object_key
		FROM game_version_assets gva
		JOIN assets a ON a.id = gva.asset_id
		JOIN game_versions v ON v.id = gva.game_version_id
		JOIN games g ON g.id = v.game_id
		WHERE gva.game_version_id = ? AND v.game_id = ? AND g.user_id = ?
		  AND g.status <> 'deleting' AND v.status = 'ready'
		  AND gva.role = 'render' AND a.kind = 'game_render'
		  AND a.internal_status IN ('ready', 'available')
		ORDER BY gva.sort_order, a.created_at, a.id`, versionID, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("list preview render assets: %w", err)
	}
	defer rows.Close()

	assets := make([]PreviewAsset, 0)
	for rows.Next() {
		var asset PreviewAsset
		if err := rows.Scan(&asset.ID, &asset.MIMEType, &asset.Bucket, &asset.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan preview render asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate preview render assets: %w", err)
	}
	return assets, nil
}

func scanVersion(row scanner) (Version, error) {
	var version Version
	err := row.Scan(
		&version.ID, &version.GameID, &version.VersionNumber, &version.Status,
		&version.InputSchemaVersion, &version.InputPayloadCiphertext, &version.InputPayloadNonce,
		&version.EncryptionKeyVersion, &version.TemplateID, &version.TemplateVersion,
		&version.CreatedAt, &version.UpdatedAt, &version.AssetCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, fmt.Errorf("scan game version: %w", err)
	}
	return version, nil
}

func (repository *Repository) AddAsset(ctx context.Context, userID, gameID, versionID string, asset Asset, replaceExisting bool, maxItems int) ([]ObjectRef, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin asset creation: %w", err)
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT v.status
		FROM game_versions v JOIN games g ON g.id = v.game_id
		WHERE v.id = ? AND v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'
		FOR UPDATE`, versionID, gameID, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock version for asset: %w", err)
	}
	if status != "draft" {
		return nil, ErrVersionNotDraft
	}

	var slotValue any
	if asset.SlotKey != "" {
		slotValue = asset.SlotKey
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id, a.bucket, a.object_key
		FROM game_version_assets gva JOIN assets a ON a.id = gva.asset_id
		WHERE gva.game_version_id = ? AND gva.slot_key <=> ?
		ORDER BY gva.sort_order, a.created_at, a.id`, versionID, slotValue)
	if err != nil {
		return nil, fmt.Errorf("list existing slot assets: %w", err)
	}
	type existingAsset struct{ id, bucket, key string }
	existing := make([]existingAsset, 0)
	for rows.Next() {
		var item existingAsset
		if err := rows.Scan(&item.id, &item.bucket, &item.key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan existing slot asset: %w", err)
		}
		existing = append(existing, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close existing slot assets: %w", err)
	}
	if !replaceExisting && maxItems > 0 && len(existing) >= maxItems {
		return nil, ErrAssetSlotFull
	}
	replaced := make([]ObjectRef, 0, len(existing))
	if replaceExisting {
		for _, item := range existing {
			if _, err := tx.ExecContext(ctx, `DELETE FROM game_version_assets WHERE game_version_id = ? AND asset_id = ?`, versionID, item.id); err != nil {
				return nil, fmt.Errorf("unlink replaced asset: %w", err)
			}
			var remainingLinks int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_version_assets WHERE asset_id = ?`, item.id).Scan(&remainingLinks); err != nil {
				return nil, fmt.Errorf("count replaced asset links: %w", err)
			}
			if remainingLinks == 0 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, item.id); err != nil {
					return nil, fmt.Errorf("delete replaced asset metadata: %w", err)
				}
				replaced = append(replaced, ObjectRef{Bucket: item.bucket, Key: item.key})
			}
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO assets
		(id, owner_user_id, kind, bucket, object_key, mime_type, size_bytes,
		 checksum_sha256, width, height, internal_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ready', ?)`,
		asset.ID, userID, asset.Kind, asset.Bucket, asset.ObjectKey, asset.MIMEType,
		asset.SizeBytes, asset.ChecksumSHA256, asset.Width, asset.Height, asset.CreatedAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("insert asset: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO game_version_assets (game_version_id, asset_id, role, slot_key, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, versionID, asset.ID, asset.Role, slotValue, asset.SortOrder, asset.CreatedAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("link version asset: %w", err)
	}
	if asset.Role == "cover" {
		if _, err := tx.ExecContext(ctx, `UPDATE games SET cover_asset_id = ? WHERE id = ?`, asset.ID, gameID); err != nil {
			return nil, fmt.Errorf("select game cover asset: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET updated_at = ? WHERE id = ?`, asset.CreatedAt.UTC(), gameID); err != nil {
		return nil, fmt.Errorf("touch game after asset creation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit asset creation: %w", err)
	}
	return replaced, nil
}

func (repository *Repository) ListAssets(ctx context.Context, userID, gameID, versionID string) ([]Asset, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT a.id, gva.game_version_id, a.kind, gva.role, COALESCE(gva.slot_key, ''), a.bucket, a.object_key,
		       a.mime_type, a.size_bytes, a.checksum_sha256, a.width, a.height,
		       gva.sort_order, a.created_at
		FROM game_version_assets gva
		JOIN assets a ON a.id = gva.asset_id
		JOIN game_versions v ON v.id = gva.game_version_id
		JOIN games g ON g.id = v.game_id
		WHERE gva.game_version_id = ? AND v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'
		  AND gva.role IN ('source', 'cover')
		ORDER BY gva.slot_key, gva.sort_order, a.created_at, a.id`, versionID, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("list version assets: %w", err)
	}
	defer rows.Close()
	assets := make([]Asset, 0)
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate version assets: %w", err)
	}
	if len(assets) == 0 {
		if _, err := repository.GetVersion(ctx, userID, gameID, versionID); err != nil {
			return nil, err
		}
	}
	return assets, nil
}

func (repository *Repository) GetAsset(ctx context.Context, userID, gameID, versionID, assetID string) (Asset, error) {
	return scanAsset(repository.db.QueryRowContext(ctx, `
		SELECT a.id, gva.game_version_id, a.kind, gva.role, COALESCE(gva.slot_key, ''), a.bucket, a.object_key,
		       a.mime_type, a.size_bytes, a.checksum_sha256, a.width, a.height,
		       gva.sort_order, a.created_at
		FROM game_version_assets gva
		JOIN assets a ON a.id = gva.asset_id
		JOIN game_versions v ON v.id = gva.game_version_id
		JOIN games g ON g.id = v.game_id
		WHERE a.id = ? AND gva.game_version_id = ? AND v.game_id = ?
		  AND g.user_id = ? AND g.status <> 'deleting' AND v.status = 'draft'`,
		assetID, versionID, gameID, userID))
}

func scanAsset(row scanner) (Asset, error) {
	var asset Asset
	err := row.Scan(
		&asset.ID, &asset.GameVersionID, &asset.Kind, &asset.Role, &asset.SlotKey, &asset.Bucket,
		&asset.ObjectKey, &asset.MIMEType, &asset.SizeBytes, &asset.ChecksumSHA256,
		&asset.Width, &asset.Height, &asset.SortOrder, &asset.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, fmt.Errorf("scan asset: %w", err)
	}
	return asset, nil
}

func (repository *Repository) ReorderAssets(ctx context.Context, userID, gameID, versionID, slotKey string, assetIDs []string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin asset reorder: %w", err)
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT v.status FROM game_versions v JOIN games g ON g.id = v.game_id
		WHERE v.id = ? AND v.game_id = ? AND g.user_id = ? AND g.status <> 'deleting'
		FOR UPDATE`, versionID, gameID, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock version for asset reorder: %w", err)
	}
	if status != "draft" {
		return ErrVersionNotDraft
	}

	rows, err := tx.QueryContext(ctx, `SELECT asset_id FROM game_version_assets WHERE game_version_id = ? AND slot_key = ?`, versionID, slotKey)
	if err != nil {
		return fmt.Errorf("list assets for reorder: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan asset for reorder: %w", err)
		}
		existing[id] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close assets for reorder: %w", err)
	}
	if len(existing) != len(assetIDs) {
		return ErrAssetOrder
	}
	seen := make(map[string]bool, len(assetIDs))
	for index, id := range assetIDs {
		if !existing[id] || seen[id] {
			return ErrAssetOrder
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx, `UPDATE game_version_assets SET sort_order = ? WHERE game_version_id = ? AND asset_id = ? AND slot_key = ?`, index, versionID, id, slotKey); err != nil {
			return fmt.Errorf("update asset order: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET updated_at = ? WHERE id = ?`, now.UTC(), gameID); err != nil {
		return fmt.Errorf("touch game after asset reorder: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit asset reorder: %w", err)
	}
	return nil
}

func (repository *Repository) DeleteAsset(ctx context.Context, userID, gameID, versionID, assetID string, now time.Time) (bool, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin asset deletion: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		DELETE gva FROM game_version_assets gva
		JOIN game_versions v ON v.id = gva.game_version_id
		JOIN games g ON g.id = v.game_id
		WHERE gva.asset_id = ? AND gva.game_version_id = ? AND v.game_id = ?
		  AND g.user_id = ? AND g.status <> 'deleting' AND v.status = 'draft'`,
		assetID, versionID, gameID, userID)
	if err != nil {
		return false, fmt.Errorf("unlink version asset: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE games SET cover_asset_id = NULL
		WHERE id = ? AND cover_asset_id = ?`, gameID, assetID); err != nil {
		return false, fmt.Errorf("clear deleted game cover: %w", err)
	}
	var remainingLinks int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_version_assets WHERE asset_id = ?`, assetID).Scan(&remainingLinks); err != nil {
		return false, fmt.Errorf("count deleted asset links: %w", err)
	}
	removeObject := remainingLinks == 0
	if removeObject {
		if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, assetID); err != nil {
			return false, fmt.Errorf("delete asset metadata: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE games SET updated_at = ? WHERE id = ?`, now.UTC(), gameID); err != nil {
		return false, fmt.Errorf("touch game after asset deletion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit asset deletion: %w", err)
	}
	return removeObject, nil
}

func (repository *Repository) RequestDeletion(ctx context.Context, jobID, userID, gameID string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin game deletion request: %w", err)
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM games WHERE id = ? AND user_id = ? FOR UPDATE`, gameID, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock game for deletion: %w", err)
	}
	if status == "deleting" {
		return ErrNotFound
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT a.id, a.bucket, a.object_key
		FROM assets a
		JOIN game_version_assets gva ON gva.asset_id = a.id
		JOIN game_versions v ON v.id = gva.game_version_id
		WHERE v.game_id = ?`, gameID)
	if err != nil {
		return fmt.Errorf("select game assets for deletion: %w", err)
	}
	payload := DeletionPayload{ObjectKeys: make([]ObjectRef, 0), AssetIDs: make([]string, 0)}
	for rows.Next() {
		var assetID, bucket, key string
		if err := rows.Scan(&assetID, &bucket, &key); err != nil {
			rows.Close()
			return fmt.Errorf("scan deletion asset: %w", err)
		}
		payload.AssetIDs = append(payload.AssetIDs, assetID)
		payload.ObjectKeys = append(payload.ObjectKeys, ObjectRef{Bucket: bucket, Key: key})
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close deletion asset rows: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode game deletion payload: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE games SET status = 'deleting', deletion_requested_at = ?, updated_at = ? WHERE id = ?`,
		now.UTC(), now.UTC(), gameID); err != nil {
		return fmt.Errorf("mark game deleting: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE share_links SET revoked_at = ?, revoke_reason = 'game_deleted'
		WHERE game_id = ? AND revoked_at IS NULL`, now.UTC(), gameID); err != nil {
		return fmt.Errorf("revoke game shares: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_generation_runs
		SET status = 'cancelled', cancel_requested_at = ?, completed_at = ?, updated_at = ?
		WHERE game_id = ? AND status IN ('queued', 'running')`, now.UTC(), now.UTC(), now.UTC(), gameID); err != nil {
		return fmt.Errorf("cancel game generations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO game_deletion_jobs
		(id, game_id, object_prefixes, status, attempt_count, next_attempt_at, created_at)
		VALUES (?, ?, ?, 'queued', 0, ?, ?)`, jobID, gameID, payloadJSON, now.UTC(), now.UTC()); err != nil {
		return fmt.Errorf("enqueue game deletion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit game deletion request: %w", err)
	}
	return nil
}

func (repository *Repository) ClaimDeletionJob(ctx context.Context, now time.Time) (DeletionJob, bool, error) {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return DeletionJob{}, false, fmt.Errorf("begin deletion job claim: %w", err)
	}
	defer tx.Rollback()

	var job DeletionJob
	var payloadJSON []byte
	err = tx.QueryRowContext(ctx, `
		SELECT id, game_id, object_prefixes
		FROM game_deletion_jobs
		WHERE status IN ('queued', 'failed') AND next_attempt_at <= ?
		ORDER BY next_attempt_at, created_at
		LIMIT 1 FOR UPDATE SKIP LOCKED`, now.UTC()).Scan(&job.ID, &job.GameID, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return DeletionJob{}, false, nil
	}
	if err != nil {
		return DeletionJob{}, false, fmt.Errorf("select deletion job: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &job.Payload); err != nil {
		return DeletionJob{}, false, fmt.Errorf("decode deletion job payload: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_deletion_jobs
		SET status = 'running', attempt_count = attempt_count + 1
		WHERE id = ?`, job.ID); err != nil {
		return DeletionJob{}, false, fmt.Errorf("mark deletion job running: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DeletionJob{}, false, fmt.Errorf("commit deletion job claim: %w", err)
	}
	return job, true, nil
}

func (repository *Repository) CompleteDeletionJob(ctx context.Context, job DeletionJob, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deletion job completion: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE games SET current_version_id = NULL, current_generation_run_id = NULL, cover_asset_id = NULL
		WHERE id = ? AND status = 'deleting'`, job.GameID); err != nil {
		return fmt.Errorf("clear game deletion pointers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM games WHERE id = ? AND status = 'deleting'`, job.GameID); err != nil {
		return fmt.Errorf("delete game records: %w", err)
	}
	for _, assetID := range job.Payload.AssetIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, assetID); err != nil {
			return fmt.Errorf("delete game asset metadata: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE game_deletion_jobs SET status = 'succeeded', completed_at = ?, last_error_code = NULL
		WHERE id = ?`, now.UTC(), job.ID); err != nil {
		return fmt.Errorf("complete deletion job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deletion job completion: %w", err)
	}
	return nil
}

func (repository *Repository) FailDeletionJob(ctx context.Context, jobID, errorCode string, nextAttempt time.Time) error {
	_, err := repository.db.ExecContext(ctx, `
		UPDATE game_deletion_jobs
		SET status = 'failed', last_error_code = ?, next_attempt_at = ?
		WHERE id = ?`, errorCode, nextAttempt.UTC(), jobID)
	if err != nil {
		return fmt.Errorf("fail deletion job: %w", err)
	}
	return nil
}
