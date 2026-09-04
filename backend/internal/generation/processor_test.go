package generation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/imagegeneration"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"
	"gamegen/backend/internal/platform/storage"
)

func TestBuildLoveJourneyConfigUsesEncryptedCreatorInputs(t *testing.T) {
	contentCipher, err := contentcrypto.New(base64.StdEncoding.EncodeToString(make([]byte, 32)), 1)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		GameTitle: "我们的旅程", TemplateID: "love-journey", TemplateVersion: "1.1.0",
	}
	plaintext := []byte(`{"sceneInputs":{"loveLetter":" 写给你的信 ","letterPassword":"0123","passwordHint":" 纪念日 "}}`)
	ciphertext, nonce, keyVersion, err := contentCipher.Encrypt(plaintext, []byte(run.GameVersionID))
	if err != nil {
		t.Fatal(err)
	}
	repository := &fakeProcessorRepository{versionInput: VersionInput{
		Ciphertext: ciphertext, Nonce: nonce, KeyVersion: keyVersion,
	}}
	processor := NewProcessor(
		repository, nil, nil, 0, analytics.NoopRecorder{}, "worker", config.GenerationConfig{}, contentCipher,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	document, err := processor.buildConfigDocument(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if document.TemplateVersion != "1.1.0" || document.Config.LoveLetter != "写给你的信" ||
		document.Config.LetterPassword != "0123" || document.Config.PasswordHint != "纪念日" {
		t.Fatalf("unexpected generated config: %#v", document)
	}
}

func TestCreateArtifactTransformsAndStoresGeneratedPNG(t *testing.T) {
	generated := testPNG(t, 2, 3)
	transformer := &fakeImageTransformer{results: []imagegeneration.Result{{Image: generated, ProviderRequestID: "provider-request"}}}
	repository := &fakeProcessorRepository{sourceAssets: []SourceAsset{{
		Bucket: "gamegen-source-assets", ObjectKey: "source/original.jpg", MIMEType: "image/jpeg",
		Width: 1200, Height: 800, SlotKey: "memoryPhotos", SortOrder: 5,
	}}}
	processor := NewProcessor(
		repository, nil, transformer, 1024, analytics.NoopRecorder{}, "worker", config.GenerationConfig{}, nil,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	processor.readObject = func(context.Context, string, string, int64) ([]byte, error) {
		return []byte("reviewed-source-image"), nil
	}
	stored := make(map[string][]byte)
	processor.putFile = func(_ context.Context, bucket, key, path, contentType string) (int64, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		stored[bucket+"/"+key] = data
		return int64(len(data)), nil
	}
	processor.removeArtifact = func(context.Context, string, string) error { return nil }
	run := Run{
		UserID: "01K00000000000000000000001", GameID: "01K00000000000000000000002",
		GameVersionID: "01K00000000000000000000003", GameTitle: "我们的旅程",
		TemplateID: "love-journey", TemplateVersion: "1.0.0",
	}
	artifact, err := processor.createAndStoreArtifact(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.RenderAssets) != 1 {
		t.Fatalf("render assets=%#v", artifact.RenderAssets)
	}
	render := artifact.RenderAssets[0]
	if render.Bucket != renderAssetsBucket || render.MIMEType != "image/png" || render.Width != 2 || render.Height != 3 ||
		render.SlotKey != "memoryPhotos" || render.SortOrder != 0 {
		t.Fatalf("unexpected generated render metadata: %#v", render)
	}
	if len(transformer.inputs) != 1 || string(transformer.inputs[0].Image) != "reviewed-source-image" ||
		!strings.Contains(transformer.inputs[0].Prompt, "Preserve the exact number of people") {
		t.Fatalf("unexpected image-to-image input: %#v", transformer.inputs)
	}
	if len(stored) != 2 {
		t.Fatalf("stored objects=%d, want config and generated PNG", len(stored))
	}
	if _, ok := stored[render.Bucket+"/"+render.ObjectKey]; !ok {
		t.Fatal("generated PNG was not written to the render-assets bucket")
	}
}

func TestCreateArtifactCleansStoredObjectsWhenTransformationFails(t *testing.T) {
	transformer := &fakeImageTransformer{
		results: []imagegeneration.Result{{Image: testPNG(t, 1, 1)}},
		errors:  []error{nil, imagegeneration.ErrUnavailable},
	}
	repository := &fakeProcessorRepository{sourceAssets: []SourceAsset{
		{Bucket: "source", ObjectKey: "one.png", MIMEType: "image/png"},
		{Bucket: "source", ObjectKey: "two.png", MIMEType: "image/png"},
	}}
	processor := NewProcessor(
		repository, nil, transformer, 1024, analytics.NoopRecorder{}, "worker", config.GenerationConfig{}, nil,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	processor.readObject = func(context.Context, string, string, int64) ([]byte, error) { return []byte("source"), nil }
	processor.putFile = func(_ context.Context, _, _, path, _ string) (int64, error) {
		info, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	}
	removed := make([]string, 0)
	processor.removeArtifact = func(_ context.Context, bucket, key string) error {
		removed = append(removed, bucket+"/"+key)
		return nil
	}
	_, err := processor.createAndStoreArtifact(context.Background(), Run{
		UserID: "user", GameID: "game", GameVersionID: "version", GameTitle: "title",
		TemplateID: "love-journey", TemplateVersion: "1.0.0",
	})
	if !errors.Is(err, imagegeneration.ErrUnavailable) {
		t.Fatalf("expected provider failure, got %v", err)
	}
	if len(removed) != 2 || !strings.Contains(removed[0], artifactsBucket) || !strings.Contains(removed[1], renderAssetsBucket) {
		t.Fatalf("unexpected cleanup calls: %#v", removed)
	}
}

func TestCreateArtifactClassifiesOversizedSourcesAsInputFailures(t *testing.T) {
	tests := []struct {
		name       string
		sourceSize int64
		readError  error
		wantReads  int
	}{
		{name: "metadata exceeds limit", sourceSize: 1025, wantReads: 0},
		{name: "storage detects limit", readError: storage.ErrReadLimitExceeded, wantReads: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transformer := &fakeImageTransformer{}
			repository := &fakeProcessorRepository{sourceAssets: []SourceAsset{{
				Bucket: "source", ObjectKey: "large.png", MIMEType: "image/png", SizeBytes: test.sourceSize,
			}}}
			processor := NewProcessor(
				repository, nil, transformer, 1024, analytics.NoopRecorder{}, "worker", config.GenerationConfig{}, nil,
				slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			)
			readCalls := 0
			processor.readObject = func(context.Context, string, string, int64) ([]byte, error) {
				readCalls++
				return nil, test.readError
			}
			processor.putFile = func(context.Context, string, string, string, string) (int64, error) { return 1, nil }
			processor.removeArtifact = func(context.Context, string, string) error { return nil }

			_, err := processor.createAndStoreArtifact(context.Background(), Run{
				UserID: "user", GameID: "game", GameVersionID: "version", GameTitle: "title",
				TemplateID: "love-journey", TemplateVersion: "1.0.0",
			})
			if !errors.Is(err, imagegeneration.ErrInputTooLarge) {
				t.Fatalf("expected input size failure, got %v", err)
			}
			if readCalls != test.wantReads || len(transformer.inputs) != 0 {
				t.Fatalf("read calls=%d transformer calls=%d", readCalls, len(transformer.inputs))
			}
		})
	}
}

func TestProcessorRecordsOnlyFinalSuccessAndFailure(t *testing.T) {
	baseRun := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 2, Status: "running",
	}
	artifact := Artifact{ID: "01K00000000000000000000005"}
	newProcessor := func(repository *fakeProcessorRepository, recorder analytics.Recorder) *Processor {
		processor := NewProcessor(repository, nil, nil, 0, recorder, "worker", config.GenerationConfig{}, nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		processor.createArtifact = func(context.Context, Run) (Artifact, error) { return artifact, nil }
		processor.removeArtifact = func(context.Context, string, string) error { return nil }
		return processor
	}

	t.Run("success", func(t *testing.T) {
		repository := &fakeProcessorRepository{run: baseRun, claimed: true}
		recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
		processed, err := newProcessor(repository, recorder).ProcessOne(context.Background())
		if err != nil || !processed || repository.successCalls != 1 || repository.failureCalls != 0 {
			t.Fatalf("process result=(%v,%v), success=%d failure=%d", processed, err, repository.successCalls, repository.failureCalls)
		}
		assertWorkerEvent(t, recorder.RecordedInputs(), analytics.EventGenerationSucceeded, "")
	})

	t.Run("final failure", func(t *testing.T) {
		repository := &fakeProcessorRepository{run: baseRun, claimed: true}
		recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
		processor := newProcessor(repository, recorder)
		processor.createArtifact = func(context.Context, Run) (Artifact, error) { return Artifact{}, imagegeneration.ErrUnavailable }
		processed, err := processor.ProcessOne(context.Background())
		if err != nil || !processed || repository.failureCalls != 1 || repository.successCalls != 0 {
			t.Fatalf("process result=(%v,%v), success=%d failure=%d", processed, err, repository.successCalls, repository.failureCalls)
		}
		assertWorkerEvent(t, recorder.RecordedInputs(), analytics.EventGenerationFailed, "PROVIDER_UNAVAILABLE")
	})

	t.Run("lease exhausted claim", func(t *testing.T) {
		failed := baseRun
		failed.Status = "failed"
		failed.ErrorCode = sql.NullString{String: "TASK_LEASE_EXHAUSTED", Valid: true}
		failed.Retryable = true
		repository := &fakeProcessorRepository{run: failed, claimed: true}
		recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
		processed, err := newProcessor(repository, recorder).ProcessOne(context.Background())
		if err != nil || !processed {
			t.Fatalf("process result=(%v,%v)", processed, err)
		}
		assertWorkerEvent(t, recorder.RecordedInputs(), analytics.EventGenerationFailed, "TASK_LEASE_EXHAUSTED")
	})
}

func TestProcessorDoesNotRecordCancellationOrTransientFailure(t *testing.T) {
	baseRun := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 1, Status: "running",
	}
	tests := []struct {
		name       string
		repository *fakeProcessorRepository
	}{
		{"cancelled", &fakeProcessorRepository{run: baseRun, claimed: true, cancelOnProgress: true}},
		{"unexhausted lease transient retry", &fakeProcessorRepository{run: baseRun, claimed: true, progressErr: errors.New("temporary database failure")}},
		{"claim error", &fakeProcessorRepository{claimErr: errors.New("temporary claim failure")}},
		{"no work", &fakeProcessorRepository{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
			processor := NewProcessor(test.repository, nil, nil, 0, recorder, "worker", config.GenerationConfig{}, nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
			processor.createArtifact = func(context.Context, Run) (Artifact, error) { return Artifact{}, nil }
			processor.removeArtifact = func(context.Context, string, string) error { return nil }
			_, _ = processor.ProcessOne(context.Background())
			if len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("non-final path recorded events: %#v", recorder.RecordedInputs())
			}
		})
	}
}

func TestProcessorHeartbeatCancelsLongImageTransformation(t *testing.T) {
	run := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 1, Status: "running",
	}
	repository := &fakeProcessorRepository{run: run, claimed: true, cancelAtProgressCall: 2}
	processor := NewProcessor(
		repository, nil, nil, 0, analytics.NoopRecorder{}, "worker",
		config.GenerationConfig{LeaseDuration: 300 * time.Millisecond, MaxExecutions: 3}, nil,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	processor.createArtifact = func(ctx context.Context, _ Run) (Artifact, error) {
		<-ctx.Done()
		return Artifact{}, ctx.Err()
	}
	processor.removeArtifact = func(context.Context, string, string) error { return nil }
	processed, err := processor.ProcessOne(context.Background())
	if err != nil || !processed || repository.cancelCalls != 1 || repository.progressCalls < 2 || repository.failureCalls != 0 {
		t.Fatalf(
			"processed=%v err=%v cancel=%d progress=%d failure=%d",
			processed, err, repository.cancelCalls, repository.progressCalls, repository.failureCalls,
		)
	}
}

func TestProcessorAnalyticsFailureDoesNotChangeSuccessfulCompletionAndDoesNotLeak(t *testing.T) {
	run := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 1, Status: "running",
	}
	repository := &fakeProcessorRepository{run: run, claimed: true}
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, errors.New("secret object path and login_id"))
	var logs bytes.Buffer
	processor := NewProcessor(repository, nil, nil, 0, recorder, "worker", config.GenerationConfig{}, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	processor.createArtifact = func(context.Context, Run) (Artifact, error) { return Artifact{ID: "artifact"}, nil }
	processor.removeArtifact = func(context.Context, string, string) error { return nil }
	processed, err := processor.ProcessOne(context.Background())
	if err != nil || !processed || repository.successCalls != 1 {
		t.Fatalf("analytics failure changed completion: processed=%v err=%v success=%d", processed, err, repository.successCalls)
	}
	if output := logs.String(); strings.Contains(output, "secret object path") || strings.Contains(output, "login_id") {
		t.Fatalf("worker analytics warning leaked details: %s", output)
	}
}

func TestProcessorPersistenceFailuresAndCancellationRaceDoNotRecordFinalEvents(t *testing.T) {
	baseRun := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 1, Status: "running",
	}
	tests := []struct {
		name        string
		successErr  error
		failureErr  error
		artifactErr error
		wantErr     bool
	}{
		{"complete success database failure", errors.New("complete failed"), nil, nil, true},
		{"cancellation won", ErrCancellationWon, nil, nil, false},
		{"persist final failure failed", nil, errors.New("fail update failed"), imagegeneration.ErrUnavailable, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeProcessorRepository{run: baseRun, claimed: true, successErr: test.successErr, failureErr: test.failureErr}
			recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
			processor := NewProcessor(repository, nil, nil, 0, recorder, "worker", config.GenerationConfig{}, nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
			processor.createArtifact = func(context.Context, Run) (Artifact, error) {
				if test.artifactErr != nil {
					return Artifact{}, test.artifactErr
				}
				return Artifact{Bucket: "bucket", ObjectKey: "object"}, nil
			}
			removeCalls := 0
			processor.removeArtifact = func(context.Context, string, string) error { removeCalls++; return nil }
			_, err := processor.ProcessOne(context.Background())
			if (err != nil) != test.wantErr || len(recorder.RecordedInputs()) != 0 {
				t.Fatalf("err=%v events=%#v", err, recorder.RecordedInputs())
			}
			if test.successErr != nil && removeCalls != 1 {
				t.Fatalf("artifact removals=%d", removeCalls)
			}
		})
	}
}

func TestProcessorFinalFailureAnalyticsFailureIsNonBlockingAndSanitized(t *testing.T) {
	run := Run{
		ID: "01K00000000000000000000004", UserID: "01K00000000000000000000001",
		GameID: "01K00000000000000000000002", GameVersionID: "01K00000000000000000000003",
		ExecutionCount: 3, Status: "running",
	}
	repository := &fakeProcessorRepository{run: run, claimed: true}
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, errors.New("secret prompt and object key"))
	var logs bytes.Buffer
	processor := NewProcessor(repository, nil, nil, 0, recorder, "worker", config.GenerationConfig{}, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	processor.createArtifact = func(context.Context, Run) (Artifact, error) { return Artifact{}, imagegeneration.ErrUnavailable }
	processor.removeArtifact = func(context.Context, string, string) error { return nil }
	processed, err := processor.ProcessOne(context.Background())
	if err != nil || !processed || repository.failureCalls != 1 || len(recorder.RecordedInputs()) != 1 {
		t.Fatalf("processed=%v err=%v failures=%d records=%d", processed, err, repository.failureCalls, len(recorder.RecordedInputs()))
	}
	if output := logs.String(); strings.Contains(output, "secret prompt") || strings.Contains(output, "object key") {
		t.Fatalf("failure warning leaked details: %s", output)
	}
}

func assertWorkerEvent(t *testing.T, inputs []analytics.RecordInput, eventName analytics.EventName, errorCode string) {
	t.Helper()
	if len(inputs) != 1 || inputs[0].EventName != eventName {
		t.Fatalf("events = %#v, want one %s", inputs, eventName)
	}
	if _, err := analytics.ValidateRecordInput(inputs[0]); err != nil {
		t.Fatalf("invalid worker event: %v", err)
	}
	var properties map[string]any
	if err := json.Unmarshal(inputs[0].Properties, &properties); err != nil {
		t.Fatal(err)
	}
	if properties["executionCount"] != float64(2) {
		t.Fatalf("executionCount=%#v", properties["executionCount"])
	}
	if eventName == analytics.EventGenerationFailed {
		if properties["errorCode"] != errorCode || properties["retryable"] != true || len(properties) != 3 {
			t.Fatalf("failure properties=%#v", properties)
		}
	} else if len(properties) != 1 {
		t.Fatalf("success properties=%#v", properties)
	}
}

type fakeProcessorRepository struct {
	run                  Run
	claimed              bool
	claimErr             error
	progressErr          error
	cancelOnProgress     bool
	cancelAtProgressCall int
	progressCalls        int
	successErr           error
	failureErr           error
	successCalls         int
	failureCalls         int
	cancelCalls          int
	versionInput         VersionInput
	versionInputErr      error
	sourceAssets         []SourceAsset
	sourceAssetsErr      error
}

type fakeImageTransformer struct {
	inputs  []imagegeneration.Input
	results []imagegeneration.Result
	errors  []error
}

func (transformer *fakeImageTransformer) Transform(_ context.Context, input imagegeneration.Input) (imagegeneration.Result, error) {
	transformer.inputs = append(transformer.inputs, input)
	index := len(transformer.inputs) - 1
	if index < len(transformer.errors) && transformer.errors[index] != nil {
		return imagegeneration.Result{}, transformer.errors[index]
	}
	if index >= len(transformer.results) {
		return imagegeneration.Result{}, imagegeneration.ErrInvalidOutput
	}
	return transformer.results[index], nil
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	canvas.Set(0, 0, color.RGBA{R: 220, G: 120, B: 90, A: 255})
	if err := png.Encode(buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func (repository *fakeProcessorRepository) Claim(context.Context, string, int, time.Duration, time.Time) (Run, bool, error) {
	return repository.run, repository.claimed, repository.claimErr
}

func (repository *fakeProcessorRepository) UpdateProgress(context.Context, string, string, string, int, time.Duration, time.Time) (bool, error) {
	repository.progressCalls++
	cancelRequested := repository.cancelOnProgress ||
		(repository.cancelAtProgressCall > 0 && repository.progressCalls >= repository.cancelAtProgressCall)
	return cancelRequested, repository.progressErr
}

func (repository *fakeProcessorRepository) CompleteCancelled(context.Context, Run, string, time.Time) error {
	repository.cancelCalls++
	return nil
}

func (repository *fakeProcessorRepository) Fail(context.Context, Run, string, Failure, time.Time) error {
	repository.failureCalls++
	return repository.failureErr
}

func (repository *fakeProcessorRepository) CompleteSuccess(context.Context, Run, string, Artifact, time.Time) error {
	repository.successCalls++
	return repository.successErr
}

func (repository *fakeProcessorRepository) LoadVersionInput(context.Context, string, string) (VersionInput, error) {
	return repository.versionInput, repository.versionInputErr
}

func (repository *fakeProcessorRepository) LoadSourceAssets(context.Context, string, string, string) ([]SourceAsset, error) {
	return repository.sourceAssets, repository.sourceAssetsErr
}
