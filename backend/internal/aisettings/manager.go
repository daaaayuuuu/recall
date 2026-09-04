package aisettings

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/security"
)

type Manager struct {
	store           Store
	base            config.AIConfig
	environment     string
	dynamicEnabled  bool
	cipher          *secretCipher
	refreshInterval time.Duration
	now             func() time.Time

	mu       sync.Mutex
	cached   config.AIConfig
	cachedAt time.Time
	hasCache bool
}

func NewManager(store Store, base config.AIConfig, environment string, dynamic config.DynamicAIConfig) (*Manager, error) {
	manager := &Manager{
		store: store, base: base, environment: environment, dynamicEnabled: dynamic.Enabled,
		refreshInterval: dynamic.RefreshInterval, now: time.Now,
	}
	if !dynamic.Enabled {
		return manager, nil
	}
	if store == nil {
		return nil, errors.New("dynamic AI settings require a store")
	}
	cipher, err := newSecretCipher(dynamic.EncryptionKeyV1, dynamic.EncryptionKeyVersion)
	if err != nil {
		return nil, err
	}
	manager.cipher = cipher
	return manager, nil
}

func (manager *Manager) DynamicEnabled() bool {
	return manager.dynamicEnabled
}

func (manager *Manager) Current(ctx context.Context) (config.AIConfig, error) {
	if !manager.dynamicEnabled {
		return manager.base, nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.hasCache && manager.now().Sub(manager.cachedAt) < manager.refreshInterval {
		return manager.cached, nil
	}
	configuration, _, err := manager.load(ctx)
	if err != nil {
		if manager.hasCache {
			return manager.cached, nil
		}
		return config.AIConfig{}, err
	}
	manager.setCache(configuration)
	return configuration, nil
}

func (manager *Manager) View(ctx context.Context) (View, error) {
	if !manager.dynamicEnabled {
		secrets := secretsFromConfig(manager.base)
		return buildView(false, Record{}, snapshotFromConfig(manager.base), secrets, "environment"), nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	configuration, view, err := manager.load(ctx)
	if err != nil {
		return View{}, err
	}
	manager.setCache(configuration)
	return view, nil
}

func (manager *Manager) Publish(
	ctx context.Context,
	expectedVersion int64,
	settings Snapshot,
	changes SecretChanges,
	adminSessionID, adminUsername, requestID string,
) (View, map[string]string, error) {
	if !manager.dynamicEnabled {
		return View{}, nil, ErrDynamicDisabled
	}
	current, err := manager.store.Current(ctx)
	if err != nil {
		return View{}, nil, err
	}
	if current.Version != expectedVersion {
		return View{}, nil, ErrVersionConflict
	}
	secrets, err := manager.plainSecrets(current)
	if err != nil {
		return View{}, nil, err
	}
	applySecretChanges(&secrets, changes)
	settings = normalizeSnapshot(settings)
	if fields := validateSnapshot(settings, secrets, manager.environment); len(fields) > 0 {
		return View{}, fields, nil
	}
	recordID, err := security.NewID()
	if err != nil {
		return View{}, nil, fmt.Errorf("generate AI settings id: %w", err)
	}
	auditID, err := security.NewID()
	if err != nil {
		return View{}, nil, fmt.Errorf("generate AI settings audit id: %w", err)
	}
	record := Record{
		ID: recordID, Version: expectedVersion + 1, Settings: settings,
		EncryptionKeyVersion: manager.cipher.keyVersion, CreatedByAdmin: adminUsername, CreatedAt: manager.now().UTC(),
	}
	if record.TextSecret, err = manager.cipher.encrypt(secrets.Text, secretAAD(recordID, CapabilityText)); err != nil {
		return View{}, nil, err
	}
	if record.ImageModerationSecret, err = manager.cipher.encrypt(secrets.ImageModeration, secretAAD(recordID, CapabilityImageModeration)); err != nil {
		return View{}, nil, err
	}
	if record.ImageToImageSecret, err = manager.cipher.encrypt(secrets.ImageToImage, secretAAD(recordID, CapabilityImageToImage)); err != nil {
		return View{}, nil, err
	}
	if err := manager.store.Save(ctx, SaveInput{
		ExpectedVersion: expectedVersion, Record: record, AdminSessionID: adminSessionID,
		AdminUsername: adminUsername, RequestID: requestID, AuditID: auditID,
	}); err != nil {
		return View{}, nil, err
	}
	configuration, err := settings.toConfig(secrets)
	if err != nil {
		return View{}, nil, err
	}
	view := buildView(true, record, settings, secrets, "admin")
	manager.mu.Lock()
	manager.setCache(configuration)
	manager.mu.Unlock()
	return view, nil, nil
}

func (manager *Manager) Preview(ctx context.Context, settings Snapshot, changes SecretChanges) (config.AIConfig, map[string]string, error) {
	current := Record{}
	if manager.dynamicEnabled {
		var err error
		current, err = manager.store.Current(ctx)
		if err != nil {
			return config.AIConfig{}, nil, err
		}
	}
	secrets, err := manager.plainSecrets(current)
	if err != nil {
		return config.AIConfig{}, nil, err
	}
	applySecretChanges(&secrets, changes)
	settings = normalizeSnapshot(settings)
	if fields := validateSnapshot(settings, secrets, manager.environment); len(fields) > 0 {
		return config.AIConfig{}, fields, nil
	}
	configuration, err := settings.toConfig(secrets)
	return configuration, nil, err
}

func (manager *Manager) load(ctx context.Context) (config.AIConfig, View, error) {
	record, err := manager.store.Current(ctx)
	if err != nil {
		return config.AIConfig{}, View{}, err
	}
	if record.Version == 0 {
		secrets := secretsFromConfig(manager.base)
		settings := snapshotFromConfig(manager.base)
		return manager.base, buildView(true, record, settings, secrets, "environment"), nil
	}
	secrets, err := manager.plainSecrets(record)
	if err != nil {
		return config.AIConfig{}, View{}, err
	}
	configuration, err := record.Settings.toConfig(secrets)
	if err != nil {
		return config.AIConfig{}, View{}, fmt.Errorf("materialize AI settings version %d: %w", record.Version, err)
	}
	return configuration, buildView(true, record, record.Settings, secrets, "admin"), nil
}

func (manager *Manager) plainSecrets(record Record) (plainSecrets, error) {
	if record.Version == 0 {
		return secretsFromConfig(manager.base), nil
	}
	text, err := manager.cipher.decrypt(record.TextSecret, secretAAD(record.ID, CapabilityText), record.EncryptionKeyVersion)
	if err != nil {
		return plainSecrets{}, err
	}
	moderation, err := manager.cipher.decrypt(record.ImageModerationSecret, secretAAD(record.ID, CapabilityImageModeration), record.EncryptionKeyVersion)
	if err != nil {
		return plainSecrets{}, err
	}
	image, err := manager.cipher.decrypt(record.ImageToImageSecret, secretAAD(record.ID, CapabilityImageToImage), record.EncryptionKeyVersion)
	if err != nil {
		return plainSecrets{}, err
	}
	return plainSecrets{Text: text, ImageModeration: moderation, ImageToImage: image}, nil
}

func (manager *Manager) setCache(configuration config.AIConfig) {
	manager.cached = configuration
	manager.cachedAt = manager.now()
	manager.hasCache = true
}

func secretsFromConfig(configuration config.AIConfig) plainSecrets {
	return plainSecrets{
		Text:            configuration.Text.APIKey,
		ImageModeration: configuration.ImageModeration.APIKey,
		ImageToImage:    configuration.ImageToImage.APIKey,
	}
}

func applySecretChanges(secrets *plainSecrets, changes SecretChanges) {
	applySecretChange(&secrets.Text, changes.Text)
	applySecretChange(&secrets.ImageModeration, changes.ImageModeration)
	applySecretChange(&secrets.ImageToImage, changes.ImageToImage)
}

func applySecretChange(target *string, change SecretChange) {
	if change.Clear {
		*target = ""
		return
	}
	if change.Value != "" {
		*target = change.Value
	}
}

func buildView(dynamic bool, record Record, settings Snapshot, secrets plainSecrets, source string) View {
	keySource := source
	view := View{
		DynamicEnabled: dynamic, Version: record.Version, Source: source, Settings: settings,
		APIKeys: APIKeyViews{
			Text:            secretStatus(secrets.Text, keySource),
			ImageModeration: secretStatus(secrets.ImageModeration, keySource),
			ImageToImage:    secretStatus(secrets.ImageToImage, keySource),
		},
		UpdatedBy: record.CreatedByAdmin,
	}
	if !record.CreatedAt.IsZero() {
		updated := record.CreatedAt.UTC()
		view.UpdatedAt = &updated
	}
	return view
}
