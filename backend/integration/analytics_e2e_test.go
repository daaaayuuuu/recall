package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const e2eOptIn = "ANALYTICS_E2E"

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (buffer *lockedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.Write(value)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.String()
}

type childProcess struct {
	command  *exec.Cmd
	exited   chan struct{}
	logs     *lockedBuffer
	stopOnce sync.Once
	errMu    sync.Mutex
	waitErr  error
}

func startProcess(t *testing.T, binary string, overrides map[string]string) *childProcess {
	return startProcessArgs(t, binary, nil, overrides, "")
}

func startProcessArgs(t *testing.T, binary string, arguments []string, overrides map[string]string, directory string) *childProcess {
	t.Helper()
	logs := &lockedBuffer{}
	command := exec.Command(binary, arguments...)
	command.Env = minimalChildEnvironment(overrides)
	command.Stdout = logs
	command.Stderr = logs
	command.Dir = directory
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatalf("start isolated service process: %v", err)
	}
	trackChildProcessGroup(t, command.Process.Pid)
	process := &childProcess{command: command, exited: make(chan struct{}), logs: logs}
	t.Cleanup(func() { process.stop(t) })
	go func() {
		err := command.Wait()
		process.errMu.Lock()
		process.waitErr = err
		process.errMu.Unlock()
		close(process.exited)
	}()
	return process
}

func trackChildProcessGroup(t *testing.T, processGroupID int) {
	t.Helper()
	path := os.Getenv("ANALYTICS_E2E_CHILD_PGID_FILE")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open isolated child process-group registry: %v", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", processGroupID); err != nil {
		_ = file.Close()
		t.Fatalf("record isolated child process group: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close isolated child process-group registry: %v", err)
	}
}

func (process *childProcess) stop(t *testing.T) {
	t.Helper()
	if process == nil || process.command == nil || process.command.Process == nil {
		return
	}
	process.stopOnce.Do(func() {
		select {
		case <-process.exited:
			return
		default:
		}
		_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGINT)
		select {
		case <-process.exited:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGKILL)
			select {
			case <-process.exited:
			case <-time.After(2 * time.Second):
				t.Fatal("isolated service process group did not stop")
			}
		}
	})
}

func (process *childProcess) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case <-process.exited:
		process.errMu.Lock()
		defer process.errMu.Unlock()
		return process.waitErr
	case <-time.After(timeout):
		return fmt.Errorf("isolated child process timed out")
	}
}

func TestChildProcessStopIsRecursiveAndIdempotent(t *testing.T) {
	process := startProcessArgs(t, "/bin/sh", []string{"-c", "sleep 30 & wait"}, nil, "")
	pid := process.command.Process.Pid
	process.stop(t)
	process.stop(t)
	if err := syscall.Kill(-pid, 0); err == nil {
		t.Fatal("child process group remained alive after idempotent stop")
	}
}

func minimalChildEnvironment(overrides map[string]string) []string {
	allowed := []string{"HOME", "LANG", "LC_ALL", "LC_CTYPE", "LOGNAME", "PATH", "SHELL", "TERM", "TMPDIR", "USER"}
	result := make([]string, 0, len(allowed)+len(overrides))
	for _, name := range allowed {
		if value, exists := os.LookupEnv(name); exists {
			result = append(result, name+"="+value)
		}
	}
	for name, value := range overrides {
		result = append(result, name+"="+value)
	}
	return result
}

func TestMinimalChildEnvironmentDoesNotInheritSecrets(t *testing.T) {
	t.Setenv("ANALYTICS_E2E_ENVIRONMENT_CANARY", "must-not-be-inherited")
	environment := minimalChildEnvironment(map[string]string{"ANALYTICS_E2E_FRONTEND_PORT": "43123"})
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "ANALYTICS_E2E_ENVIRONMENT_CANARY") || strings.Contains(joined, "must-not-be-inherited") {
		t.Fatal("minimal child environment inherited an unrelated parent variable")
	}
	if !strings.Contains(joined, "ANALYTICS_E2E_FRONTEND_PORT=43123") {
		t.Fatal("minimal child environment omitted an explicit override")
	}
}

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

type testRuntime struct {
	t                   *testing.T
	baseURL             string
	origin              string
	db                  *sql.DB
	creator             *http.Client
	play                *http.Client
	admin               *http.Client
	csrf                string
	apiLogs             *lockedBuffer
	viteLogs            *lockedBuffer
	creatorID           string
	loginID             string
	password            string
	creatorPasswordHash string
	memoryText          string
	fileName            string
	imageBase64         string
	artifactCanaries    []string
	shareSecrets        []string
	invitationCodes     []string
}

func TestAnalyticsE2E(t *testing.T) {
	if os.Getenv(e2eOptIn) != "1" {
		t.Skip("set ANALYTICS_E2E=1 and use scripts/test-analytics-e2e.sh")
	}
	requireEnv(t,
		"ANALYTICS_E2E_MYSQL_DSN", "ANALYTICS_E2E_API_BIN", "ANALYTICS_E2E_WORKER_BIN",
		"ANALYTICS_E2E_ADMIN_USERNAME", "ANALYTICS_E2E_ADMIN_PASSWORD",
		"ANALYTICS_E2E_ADMIN_PASSWORD_HASH", "ANALYTICS_E2E_CREATOR_PASSWORD",
		"ANALYTICS_E2E_FRONTEND_DIR", "ANALYTICS_E2E_BROWSER_SCRIPT", "ANALYTICS_E2E_BROWSER_GENERATOR",
		"ANALYTICS_E2E_BROWSER_TASK", "ANALYTICS_E2E_CHILD_PGID_FILE",
	)

	db, err := sql.Open("mysql", os.Getenv("ANALYTICS_E2E_MYSQL_DSN"))
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated database: %v", err)
	}

	apiPort := freePort(t)
	frontendPort := freePort(t)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort)
	frontendURL := "http://127.0.0.1:" + strconv.Itoa(frontendPort)
	t.Setenv("ANALYTICS_E2E_API_PORT", strconv.Itoa(apiPort))
	t.Setenv("ANALYTICS_E2E_FRONTEND_PORT", strconv.Itoa(frontendPort))
	t.Setenv("ANALYTICS_E2E_FRONTEND_URL", frontendURL)
	api := startProcess(t, os.Getenv("ANALYTICS_E2E_API_BIN"), processEnvironment("all", apiPort, freePort(t), false))
	waitReady(t, baseURL+"/health/ready", api)
	vite := startProcessArgs(t, "npm", []string{"run", "dev", "--", "--config", "vite.e2e.config.ts"},
		map[string]string{
			"ANALYTICS_E2E_API_PORT":      strconv.Itoa(apiPort),
			"ANALYTICS_E2E_FRONTEND_PORT": strconv.Itoa(frontendPort),
		}, os.Getenv("ANALYTICS_E2E_FRONTEND_DIR"))
	waitReady(t, frontendURL, vite)
	if os.Getenv("ANALYTICS_E2E_FORCE_FAILURE_AFTER_START") == "1" {
		t.Fatal("intentional E2E lifecycle failure after API and Vite startup")
	}

	creator := clientWithJar(t)
	play := clientWithJar(t)
	admin := clientWithJar(t)
	runtime := &testRuntime{
		t: t, baseURL: baseURL, origin: frontendURL, db: db, creator: creator, play: play, admin: admin,
		apiLogs: api.logs, loginID: "qa_" + strings.ToLower(randomHex(t, 6)),
		viteLogs:   vite.logs,
		password:   os.Getenv("ANALYTICS_E2E_CREATOR_PASSWORD"),
		memoryText: "memory-canary-" + randomHex(t, 16), fileName: "image-canary-" + randomHex(t, 12) + ".png",
	}

	startedAt := time.Now().UTC().Add(-time.Second)
	runtime.registerAndLogin()
	mainGameID, readyVersionID := runtime.createReadyGame()
	runtime.createFailedGeneration()
	share := runtime.exercisePublicPlay(mainGameID)
	runtime.exerciseFrontendValidation()
	runtime.exerciseBrowserFrontend(mainGameID, share)
	runtime.exerciseAdminPaginationAndPrivacy(startedAt, mainGameID, share.secret)
	runtime.exerciseInvalidSharesAndDeletion(mainGameID, share)
	runtime.exerciseAnalyticsOutage()
	runtime.exerciseSurfaceMatrix()
	runtime.assertCompleteEventSet()
	runtime.assertSnapshotsAndPrivacy(mainGameID, readyVersionID)
	runtime.assertLogsPrivate(share.secret)
}

func requireEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("required E2E environment variable %s is missing", name)
		}
	}
}

func processEnvironment(surface string, apiPort, workerPort int, failImageGeneration bool) map[string]string {
	frontendURL := os.Getenv("ANALYTICS_E2E_FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://127.0.0.1:5173"
	}
	environment := map[string]string{
		"APP_ENVIRONMENT": "test", "SERVICE_SURFACE": surface,
		"APP_BASE_URL": frontendURL, "PLAY_BASE_URL": frontendURL,
		"HTTP_ADDRESS":          ":" + strconv.Itoa(apiPort),
		"WORKER_HEALTH_ADDRESS": ":" + strconv.Itoa(workerPort),
		"WORKER_POLL_INTERVAL":  "20ms", "GENERATION_LEASE_DURATION": "5s",
		"GENERATION_MAX_EXECUTIONS": "1", "LOG_LEVEL": "debug",
		"MYSQL_DSN":                         os.Getenv("ANALYTICS_E2E_MYSQL_DSN"),
		"ADMIN_USERNAME":                    os.Getenv("ANALYTICS_E2E_ADMIN_USERNAME"),
		"ADMIN_PASSWORD_HASH":               os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD_HASH"),
		"MINIO_ENDPOINT":                    os.Getenv("MINIO_ENDPOINT"),
		"MINIO_PUBLIC_ENDPOINT":             os.Getenv("MINIO_PUBLIC_ENDPOINT"),
		"MINIO_ACCESS_KEY":                  os.Getenv("MINIO_ACCESS_KEY"),
		"MINIO_SECRET_KEY":                  os.Getenv("MINIO_SECRET_KEY"),
		"MINIO_REGION":                      os.Getenv("MINIO_REGION"),
		"MINIO_USE_SSL":                     os.Getenv("MINIO_USE_SSL"),
		"MINIO_PUBLIC_USE_SSL":              os.Getenv("MINIO_PUBLIC_USE_SSL"),
		"CONTENT_ENCRYPTION_KEY_V1":         os.Getenv("CONTENT_ENCRYPTION_KEY_V1"),
		"SHARE_ENCRYPTION_KEY_V1":           os.Getenv("SHARE_ENCRYPTION_KEY_V1"),
		"AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES": "26214400", "AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES": "26214400",
	}
	if failImageGeneration {
		environment["AI_IMAGE_TO_IMAGE_PROVIDER"] = "openai-compatible"
		environment["AI_IMAGE_TO_IMAGE_BASE_URL"] = "http://127.0.0.1:1/v1"
		environment["AI_IMAGE_TO_IMAGE_API_KEY"] = "intentional-e2e-failure"
		environment["AI_IMAGE_TO_IMAGE_MODEL"] = "unreachable-image-model"
		environment["AI_IMAGE_TO_IMAGE_TIMEOUT"] = "100ms"
	}
	return environment
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitReady(t *testing.T, endpoint string, process *childProcess) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(endpoint) // #nosec G107 -- loopback URL selected by the test.
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("isolated service did not become ready")
}

func clientWithJar(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create private cookie jar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

func (runtime *testRuntime) request(client *http.Client, method, path string, payload any, headers map[string]string, expected ...int) json.RawMessage {
	runtime.t.Helper()
	var body io.Reader
	if payload != nil {
		switch value := payload.(type) {
		case []byte:
			body = bytes.NewReader(value)
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				runtime.t.Fatalf("encode E2E request: %v", err)
			}
			body = bytes.NewReader(encoded)
		}
	}
	request, err := http.NewRequest(method, runtime.baseURL+path, body)
	if err != nil {
		runtime.t.Fatalf("create E2E request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		runtime.t.Fatalf("perform E2E request: %v", err)
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		runtime.t.Fatalf("read E2E response: %v", err)
	}
	matched := false
	for _, status := range expected {
		matched = matched || response.StatusCode == status
	}
	if !matched {
		runtime.t.Fatalf("E2E request %s %s returned status %d", method, stablePath(path), response.StatusCode)
	}
	if response.StatusCode == http.StatusNoContent || len(encoded) == 0 {
		return nil
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		runtime.t.Fatalf("decode E2E response envelope: %v", err)
	}
	return envelope.Data
}

func (runtime *testRuntime) requestRaw(client *http.Client, method, path string, payload []byte, headers map[string]string, expected ...int) []byte {
	runtime.t.Helper()
	request, err := http.NewRequest(method, runtime.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		runtime.t.Fatalf("create raw E2E request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		runtime.t.Fatalf("perform raw E2E request: %v", err)
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		runtime.t.Fatalf("read raw E2E response: %v", err)
	}
	for _, status := range expected {
		if response.StatusCode == status {
			return encoded
		}
	}
	runtime.t.Fatalf("raw E2E request %s %s returned status %d", method, stablePath(path), response.StatusCode)
	return nil
}

func stablePath(value string) string {
	if index := strings.IndexByte(value, '?'); index >= 0 {
		return value[:index]
	}
	return value
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode typed E2E data: %v", err)
	}
	return value
}

func (runtime *testRuntime) creatorHeaders() map[string]string {
	return map[string]string{"Origin": runtime.origin, "X-CSRF-Token": runtime.csrf}
}

func (runtime *testRuntime) registerAndLogin() {
	runtime.loginAdmin()
	adminCSRF := runtime.request(runtime.admin, http.MethodPost, "/api/v1/admin/auth/csrf", nil,
		map[string]string{"Origin": runtime.origin}, http.StatusOK)
	adminToken := decode[struct {
		Token string `json:"csrfToken"`
	}](runtime.t, adminCSRF).Token
	createdInvitation := runtime.request(runtime.admin, http.MethodPost, "/api/v1/admin/invitation-codes", nil,
		map[string]string{"Origin": runtime.origin, "X-CSRF-Token": adminToken}, http.StatusCreated)
	invitationCode := decode[struct {
		Code string `json:"code"`
	}](runtime.t, createdInvitation).Code
	if len(invitationCode) != 9 || invitationCode[4] != '-' {
		runtime.t.Fatalf("generated invitation format = %q", invitationCode)
	}
	runtime.invitationCodes = append(runtime.invitationCodes, invitationCode)

	register := runtime.request(runtime.creator, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"invitationCode": invitationCode,
		"userId":         runtime.loginID, "password": runtime.password, "nickname": "QA Creator",
	}, map[string]string{"Origin": runtime.origin}, http.StatusCreated)
	registered := decode[struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}](runtime.t, register)
	runtime.creatorID = registered.User.ID
	runtime.request(runtime.creator, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"invitationCode": invitationCode,
		"userId":         runtime.loginID + "_second", "password": runtime.password, "nickname": "Second Creator",
	}, map[string]string{"Origin": runtime.origin}, http.StatusUnprocessableEntity)
	var invitationUses int
	if err := runtime.db.QueryRow(`
		SELECT COUNT(*) FROM registration_invites
		WHERE used_by_user_id = ? AND used_at IS NOT NULL AND revoked_at IS NULL`, runtime.creatorID).Scan(&invitationUses); err != nil {
		runtime.t.Fatalf("verify invitation consumption: %v", err)
	}
	if invitationUses != 1 {
		runtime.t.Fatalf("invitation use count = %d, want 1", invitationUses)
	}
	listResponse := runtime.requestRaw(runtime.admin, http.MethodGet, "/api/v1/admin/invitation-codes?limit=50", nil, nil, http.StatusOK)
	if bytes.Contains(listResponse, []byte(invitationCode)) {
		runtime.t.Fatal("administrator invitation list exposed a full invitation code")
	}
	runtime.exerciseConcurrentInvitationUse(adminToken)
	if err := runtime.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, runtime.creatorID).Scan(&runtime.creatorPasswordHash); err != nil {
		runtime.t.Fatalf("read creator password-hash privacy canary: %v", err)
	}

	runtime.request(runtime.creator, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"userId": runtime.loginID, "password": runtime.password,
	}, map[string]string{"Origin": runtime.origin}, http.StatusOK)
	csrf := runtime.request(runtime.creator, http.MethodPost, "/api/v1/auth/csrf", nil,
		map[string]string{"Origin": runtime.origin}, http.StatusOK)
	runtime.csrf = decode[struct {
		Token string `json:"csrfToken"`
	}](runtime.t, csrf).Token
	if runtime.csrf == "" {
		runtime.t.Fatal("creator CSRF token was empty")
	}
}

func (runtime *testRuntime) exerciseConcurrentInvitationUse(adminCSRF string) {
	runtime.t.Helper()
	created := runtime.request(runtime.admin, http.MethodPost, "/api/v1/admin/invitation-codes", nil,
		map[string]string{"Origin": runtime.origin, "X-CSRF-Token": adminCSRF}, http.StatusCreated)
	code := decode[struct {
		Code string `json:"code"`
	}](runtime.t, created).Code
	runtime.invitationCodes = append(runtime.invitationCodes, code)

	loginIDs := []string{"race_a_" + strings.ToLower(randomHex(runtime.t, 4)), "race_b_" + strings.ToLower(randomHex(runtime.t, 4))}
	type result struct {
		status int
		err    error
	}
	results := make(chan result, len(loginIDs))
	start := make(chan struct{})
	for _, loginID := range loginIDs {
		loginID := loginID
		go func() {
			<-start
			payload, _ := json.Marshal(map[string]any{
				"invitationCode": code, "userId": loginID,
				"password": runtime.password, "nickname": "Race Creator",
			})
			request, err := http.NewRequest(http.MethodPost, runtime.baseURL+"/api/v1/auth/register", bytes.NewReader(payload))
			if err != nil {
				results <- result{err: err}
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", runtime.origin)
			response, err := runtime.creator.Do(request)
			if err != nil {
				results <- result{err: err}
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			results <- result{status: response.StatusCode}
		}()
	}
	close(start)
	statuses := make(map[int]int)
	for range loginIDs {
		outcome := <-results
		if outcome.err != nil {
			runtime.t.Fatalf("concurrent registration request failed: %v", outcome.err)
		}
		statuses[outcome.status]++
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusUnprocessableEntity] != 1 {
		runtime.t.Fatalf("concurrent invitation statuses = %#v", statuses)
	}

	var createdUsers, consumedInvitations int
	if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM users WHERE login_id IN (?, ?)`, loginIDs[0], loginIDs[1]).Scan(&createdUsers); err != nil {
		runtime.t.Fatalf("count concurrent registration users: %v", err)
	}
	if err := runtime.db.QueryRow(`
		SELECT COUNT(*) FROM registration_invites i
		JOIN users u ON u.id = i.used_by_user_id
		WHERE u.login_id IN (?, ?) AND i.used_at IS NOT NULL`, loginIDs[0], loginIDs[1]).Scan(&consumedInvitations); err != nil {
		runtime.t.Fatalf("count concurrent invitation uses: %v", err)
	}
	if createdUsers != 1 || consumedInvitations != 1 {
		runtime.t.Fatalf("concurrent result users=%d invitations=%d", createdUsers, consumedInvitations)
	}

	if _, err := runtime.db.Exec(`DELETE FROM behavior_events WHERE user_id IN (SELECT id FROM users WHERE login_id IN (?, ?))`, loginIDs[0], loginIDs[1]); err != nil {
		runtime.t.Fatalf("clean concurrent behavior fixture: %v", err)
	}
	if _, err := runtime.db.Exec(`DELETE FROM registration_invites WHERE used_by_user_id IN (SELECT id FROM users WHERE login_id IN (?, ?))`, loginIDs[0], loginIDs[1]); err != nil {
		runtime.t.Fatalf("clean concurrent invitation fixture: %v", err)
	}
	if _, err := runtime.db.Exec(`DELETE FROM users WHERE login_id IN (?, ?)`, loginIDs[0], loginIDs[1]); err != nil {
		runtime.t.Fatalf("clean concurrent user fixture: %v", err)
	}
}

func (runtime *testRuntime) createGame() (string, string) {
	raw := runtime.request(runtime.creator, http.MethodPost, "/api/v1/games", map[string]any{
		"title": "Analytics QA", "description": "isolated",
		"templateId": "love-journey", "templateVersion": "1.1.0",
		"sceneInputs": map[string]any{
			"loveLetter": runtime.memoryText, "letterPassword": "2580", "passwordHint": "four digits",
		},
	}, runtime.creatorHeaders(), http.StatusCreated)
	data := decode[struct {
		Game struct {
			ID string `json:"id"`
		} `json:"game"`
		Version struct {
			ID string `json:"id"`
		} `json:"version"`
	}](runtime.t, raw)
	return data.Game.ID, data.Version.ID
}

func (runtime *testRuntime) createVersion(gameID string) string {
	raw := runtime.request(runtime.creator, http.MethodPost, "/api/v1/games/"+gameID+"/versions", map[string]any{
		"sceneInputs": map[string]any{
			"loveLetter": runtime.memoryText, "letterPassword": "2580", "passwordHint": "four digits",
		},
	}, runtime.creatorHeaders(), http.StatusCreated)
	return decode[struct {
		ID string `json:"id"`
	}](runtime.t, raw).ID
}

func (runtime *testRuntime) uploadRequiredAssets(gameID, versionID string) {
	runtime.uploadAsset(gameID, versionID, "cover", 0)
	for index := 0; index < 2; index++ {
		runtime.uploadAsset(gameID, versionID, "travelPhotos", index)
	}
}

func (runtime *testRuntime) uploadAsset(gameID, versionID, slotKey string, sortOrder int) {
	runtime.t.Helper()
	var imageBuffer bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 120, A: 255})
		}
	}
	if err := png.Encode(&imageBuffer, canvas); err != nil {
		runtime.t.Fatalf("encode E2E image: %v", err)
	}
	runtime.imageBase64 = base64.StdEncoding.EncodeToString(imageBuffer.Bytes())
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, err := writer.CreateFormFile("file", runtime.fileName)
	if err != nil {
		runtime.t.Fatalf("create E2E multipart file: %v", err)
	}
	if _, err := part.Write(imageBuffer.Bytes()); err != nil {
		runtime.t.Fatalf("write E2E multipart file: %v", err)
	}
	_ = writer.WriteField("role", "source")
	_ = writer.WriteField("slotKey", slotKey)
	_ = writer.WriteField("sortOrder", strconv.Itoa(sortOrder))
	if err := writer.Close(); err != nil {
		runtime.t.Fatalf("finish E2E multipart body: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost,
		runtime.baseURL+"/api/v1/games/"+gameID+"/versions/"+versionID+"/assets", &multipartBody)
	if err != nil {
		runtime.t.Fatalf("create E2E upload request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	for name, value := range runtime.creatorHeaders() {
		request.Header.Set(name, value)
	}
	response, err := runtime.creator.Do(request)
	if err != nil {
		runtime.t.Fatalf("perform E2E upload: %v", err)
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		runtime.t.Fatalf("read E2E upload response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		runtime.t.Fatalf("E2E upload returned status %d", response.StatusCode)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		runtime.t.Fatalf("decode E2E upload response: %v", err)
	}
	asset := decode[struct {
		ID         string `json:"id"`
		PreviewURL string `json:"previewUrl"`
	}](runtime.t, envelope.Data)
	var bucket, objectKey string
	if err := runtime.db.QueryRow(`SELECT bucket, object_key FROM assets WHERE id = ?`, asset.ID).Scan(&bucket, &objectKey); err != nil {
		runtime.t.Fatalf("read uploaded artifact privacy canaries: %v", err)
	}
	runtime.artifactCanaries = append(runtime.artifactCanaries, bucket, objectKey, asset.PreviewURL)
}

func (runtime *testRuntime) submitGeneration(gameID, versionID string) string {
	raw := runtime.request(runtime.creator, http.MethodPost, "/api/v1/games/"+gameID+"/generation-runs",
		map[string]any{"versionId": versionID}, map[string]string{
			"Origin": runtime.origin, "X-CSRF-Token": runtime.csrf, "Idempotency-Key": uuidV4(runtime.t),
		}, http.StatusCreated)
	return decode[struct {
		ID string `json:"id"`
	}](runtime.t, raw).ID
}

func (runtime *testRuntime) waitRun(gameID, runID, status string) {
	runtime.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		raw := runtime.request(runtime.creator, http.MethodGet,
			"/api/v1/games/"+gameID+"/generation-runs/"+runID, nil, nil, http.StatusOK)
		current := decode[struct {
			Status string `json:"status"`
		}](runtime.t, raw)
		if current.Status == status {
			eventName := "generation." + status
			var count int
			if err := runtime.db.QueryRow(`
				SELECT COUNT(*) FROM behavior_events
				WHERE generation_run_id = ? AND event_name = ?`, runID, eventName).Scan(&count); err != nil {
				runtime.t.Fatalf("count terminal generation event: %v", err)
			}
			if count > 0 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	runtime.t.Fatalf("generation run did not reach %s", status)
}

func (runtime *testRuntime) runWorker(failImageGeneration bool, action func()) *lockedBuffer {
	runtime.t.Helper()
	port := freePort(runtime.t)
	worker := startProcess(runtime.t, os.Getenv("ANALYTICS_E2E_WORKER_BIN"),
		processEnvironment("all", freePort(runtime.t), port, failImageGeneration))
	waitReady(runtime.t, "http://127.0.0.1:"+strconv.Itoa(port)+"/health/ready", worker)
	action()
	worker.stop(runtime.t)
	return worker.logs
}

func (runtime *testRuntime) createReadyGame() (string, string) {
	gameID, _ := runtime.createGame()
	versionID := runtime.createVersion(gameID)
	runtime.uploadRequiredAssets(gameID, versionID)
	runID := runtime.submitGeneration(gameID, versionID)
	logs := runtime.runWorker(false, func() { runtime.waitRun(gameID, runID, "succeeded") })
	runtime.assertLogCanaries(logs.String(), "")
	return gameID, versionID
}

func (runtime *testRuntime) createFailedGeneration() {
	gameID, versionID := runtime.createGame()
	runtime.uploadRequiredAssets(gameID, versionID)
	runID := runtime.submitGeneration(gameID, versionID)
	logs := runtime.runWorker(true, func() { runtime.waitRun(gameID, runID, "failed") })
	runtime.assertLogCanaries(logs.String(), "")
}

type shareFixture struct {
	id       string
	publicID string
	secret   string
}

func (runtime *testRuntime) createShare(gameID string) shareFixture {
	raw := runtime.request(runtime.creator, http.MethodPost, "/api/v1/games/"+gameID+"/share-links", map[string]any{
		"expiresAt": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}, runtime.creatorHeaders(), http.StatusCreated)
	data := decode[struct {
		ID       string `json:"id"`
		PublicID string `json:"publicId"`
		URL      string `json:"url"`
	}](runtime.t, raw)
	parsed, err := url.Parse(data.URL)
	if err != nil {
		runtime.t.Fatalf("parse private share URL: %v", err)
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil || values.Get("t") == "" {
		runtime.t.Fatal("private share URL did not contain an isolated fragment credential")
	}
	secret := values.Get("t")
	runtime.shareSecrets = append(runtime.shareSecrets, secret)
	return shareFixture{id: data.ID, publicID: data.PublicID, secret: secret}
}

func (runtime *testRuntime) exercisePublicPlay(gameID string) shareFixture {
	share := runtime.createShare(gameID)
	headers := map[string]string{"Origin": runtime.origin}
	payload := map[string]any{"secret": share.secret}
	resolveData := runtime.request(runtime.play, http.MethodPost, "/api/v1/public/shares/"+share.publicID+"/resolve",
		payload, headers, http.StatusOK)
	runtime.assertResponsePrivate(resolveData, share.secret, false)
	sessionData := runtime.request(runtime.play, http.MethodPost, "/api/v1/public/shares/"+share.publicID+"/play-sessions",
		payload, headers, http.StatusCreated)
	runtime.assertResponsePrivate(sessionData, share.secret, false)

	completeID := uuidV4(runtime.t)
	complete := map[string]any{"eventName": "play.completed", "clientEventId": completeID,
		"occurredAt": time.Now().UTC().Format(time.RFC3339Nano), "properties": map[string]any{"mode": "public"}}
	runtime.request(runtime.play, http.MethodPost, "/api/v1/public/play-sessions/current/events",
		complete, headers, http.StatusCreated)
	runtime.request(runtime.play, http.MethodPost, "/api/v1/public/play-sessions/current/events",
		complete, headers, http.StatusOK)
	var count int
	if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM behavior_events WHERE client_event_id = ?`, completeID).Scan(&count); err != nil {
		runtime.t.Fatalf("count idempotent public event: %v", err)
	}
	if count != 1 {
		runtime.t.Fatalf("idempotent public event count=%d, want 1", count)
	}
	runtime.request(runtime.play, http.MethodPost, "/api/v1/public/play-sessions/current/events", map[string]any{
		"eventName": "play.replayed", "clientEventId": uuidV4(runtime.t),
		"occurredAt": time.Now().UTC().Format(time.RFC3339Nano), "properties": map[string]any{"mode": "public"},
	}, headers, http.StatusCreated)
	return share
}

func (runtime *testRuntime) exerciseBrowserFrontend(gameID string, share shareFixture) {
	runtime.t.Helper()
	before := runtime.browserFrontendEventCounts(gameID)
	rows, err := runtime.db.Query(`SELECT bucket, object_key FROM assets ORDER BY id`)
	if err != nil {
		runtime.t.Fatalf("list browser privacy artifact canaries: %v", err)
	}
	for rows.Next() {
		var bucket, objectKey string
		if err := rows.Scan(&bucket, &objectKey); err != nil {
			rows.Close()
			runtime.t.Fatalf("scan browser privacy artifact canaries: %v", err)
		}
		runtime.artifactCanaries = append(runtime.artifactCanaries, bucket, objectKey)
	}
	if err := rows.Close(); err != nil {
		runtime.t.Fatalf("close browser privacy artifact canaries: %v", err)
	}
	forbidden, err := json.Marshal(runtime.privacyCanaries(share.secret, true))
	if err != nil {
		runtime.t.Fatalf("encode private browser canaries: %v", err)
	}
	taskName := os.Getenv("ANALYTICS_E2E_BROWSER_TASK")
	shareURL := os.Getenv("ANALYTICS_E2E_FRONTEND_URL") + "/play/" + share.publicID + "#t=" + url.QueryEscape(share.secret)
	process := startProcess(runtime.t, os.Getenv("ANALYTICS_E2E_BROWSER_SCRIPT"), map[string]string{
		"ANALYTICS_E2E_BROWSER_TASK":      taskName,
		"ANALYTICS_E2E_FRONTEND_URL":      os.Getenv("ANALYTICS_E2E_FRONTEND_URL"),
		"ANALYTICS_E2E_LOGIN_ID":          runtime.loginID,
		"ANALYTICS_E2E_CREATOR_PASSWORD":  runtime.password,
		"ANALYTICS_E2E_SHARE_URL":         shareURL,
		"ANALYTICS_E2E_ADMIN_USERNAME":    os.Getenv("ANALYTICS_E2E_ADMIN_USERNAME"),
		"ANALYTICS_E2E_ADMIN_PASSWORD":    os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD"),
		"ANALYTICS_E2E_BROWSER_FORBIDDEN": string(forbidden),
		"ANALYTICS_E2E_BROWSER_GENERATOR": os.Getenv("ANALYTICS_E2E_BROWSER_GENERATOR"),
	})
	if err := process.wait(runtime.t, 90*time.Second); err != nil {
		runtime.t.Fatalf("real browser analytics flow failed: %s", runtime.sanitizedDiagnostics(process.logs.String(), share.secret))
	}
	logs := process.logs.String()
	if !strings.Contains(logs, "BROWSER_E2E_PASS") || !strings.Contains(logs, "BROWSER_TASK_CLEANUP_PASS") {
		runtime.t.Fatal("real browser analytics flow did not confirm execution and task-space cleanup")
	}
	runtime.assertLogCanaries(logs, share.secret)
	after := runtime.browserFrontendEventCounts(gameID)
	for _, key := range []string{"creator:create", "creator:games", "play:play.completed", "play:play.replayed"} {
		if after[key] <= before[key] {
			runtime.t.Fatalf("real browser UI did not add expected frontend event %s: before=%d after=%d", key, before[key], after[key])
		}
	}
}

func (runtime *testRuntime) browserFrontendEventCounts(gameID string) map[string]int {
	runtime.t.Helper()
	counts := map[string]int{}
	for _, page := range []string{"create", "games"} {
		var count int
		if err := runtime.db.QueryRow(`
			SELECT COUNT(*) FROM behavior_events
			WHERE event_name = 'creator.page_viewed' AND source = 'frontend' AND user_id = ?
			  AND JSON_UNQUOTE(JSON_EXTRACT(properties, '$.page')) = ?`, runtime.creatorID, page).Scan(&count); err != nil {
			runtime.t.Fatalf("count browser creator page %s: %v", page, err)
		}
		counts["creator:"+page] = count
	}
	for _, eventName := range []string{"play.completed", "play.replayed"} {
		var count int
		if err := runtime.db.QueryRow(`
			SELECT COUNT(*) FROM behavior_events
			WHERE event_name = ? AND source = 'frontend' AND game_id = ?
			  AND JSON_UNQUOTE(JSON_EXTRACT(properties, '$.mode')) = 'public'`, eventName, gameID).Scan(&count); err != nil {
			runtime.t.Fatalf("count browser play event %s: %v", eventName, err)
		}
		counts["play:"+eventName] = count
	}
	return counts
}

func (runtime *testRuntime) sanitizedDiagnostics(logs, secret string) string {
	for _, canary := range runtime.privacyCanaries(secret, false) {
		if canary != "" {
			logs = strings.ReplaceAll(logs, canary, "[REDACTED]")
		}
	}
	if len(logs) > 4096 {
		logs = logs[len(logs)-4096:]
	}
	return strings.TrimSpace(logs)
}

func (runtime *testRuntime) exerciseFrontendValidation() {
	spoofFields := map[string]string{
		"creatorId": runtime.creatorID, "loginId": runtime.loginID,
		"gameId": "01K00000000000000000000001", "shareId": "01K00000000000000000000002",
		"playSessionId": "01K00000000000000000000003",
	}
	for field, value := range spoofFields {
		before := runtime.behaviorCount()
		body := map[string]any{
			"eventName": "creator.page_viewed", "clientEventId": uuidV4(runtime.t),
			"properties": map[string]any{"page": "games"}, field: value,
		}
		runtime.request(runtime.creator, http.MethodPost, "/api/v1/analytics/events", body,
			runtime.creatorHeaders(), http.StatusBadRequest)
		if after := runtime.behaviorCount(); after != before {
			runtime.t.Fatalf("creator spoof field %s changed behavior count", field)
		}
	}
	for field, value := range spoofFields {
		before := runtime.behaviorCount()
		body := map[string]any{
			"eventName": "play.replayed", "clientEventId": uuidV4(runtime.t),
			"properties": map[string]any{"mode": "public"}, field: value,
		}
		runtime.request(runtime.play, http.MethodPost, "/api/v1/public/play-sessions/current/events", body,
			map[string]string{"Origin": runtime.origin}, http.StatusBadRequest)
		if after := runtime.behaviorCount(); after != before {
			runtime.t.Fatalf("public spoof field %s changed behavior count", field)
		}
	}

	runtime.request(runtime.creator, http.MethodPost, "/api/v1/analytics/events", map[string]any{
		"eventName": "unknown.event", "clientEventId": uuidV4(runtime.t), "properties": map[string]any{},
	}, runtime.creatorHeaders(), http.StatusUnprocessableEntity)
	unknown := make(map[string]any, 150)
	for index := 0; index < 150; index++ {
		unknown[fmt.Sprintf("unknown_%03d", index)] = strings.Repeat("x", 20)
	}
	runtime.request(runtime.creator, http.MethodPost, "/api/v1/analytics/events", map[string]any{
		"eventName": "creator.page_viewed", "clientEventId": uuidV4(runtime.t), "properties": unknown,
	}, runtime.creatorHeaders(), http.StatusUnprocessableEntity)
	oversized := []byte(`{"eventName":"creator.page_viewed","clientEventId":"` + uuidV4(runtime.t) +
		`","properties":{"page":"games","padding":"` + strings.Repeat("x", (1<<20)+1) + `"}}`)
	runtime.request(runtime.creator, http.MethodPost, "/api/v1/analytics/events", oversized,
		runtime.creatorHeaders(), http.StatusBadRequest)

	duplicateCanary := "duplicate-json-canary-" + randomHex(runtime.t, 8)
	duplicate := []byte(`{"eventName":"creator.page_viewed","clientEventId":"` + uuidV4(runtime.t) +
		`","properties":{"page":"games","` + duplicateCanary + `":true,"` + duplicateCanary + `":false}}`)
	response := runtime.requestRaw(runtime.creator, http.MethodPost, "/api/v1/analytics/events", duplicate,
		runtime.creatorHeaders(), http.StatusUnprocessableEntity)
	if bytes.Contains(response, []byte(duplicateCanary)) {
		runtime.t.Fatal("duplicate JSON-key canary was reflected in the API response")
	}
}

func (runtime *testRuntime) behaviorCount() int {
	runtime.t.Helper()
	var count int
	if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM behavior_events`).Scan(&count); err != nil {
		runtime.t.Fatalf("count behavior events: %v", err)
	}
	return count
}

type adminEvent struct {
	ID         string          `json:"id"`
	EventName  string          `json:"eventName"`
	LoginID    *string         `json:"loginId"`
	GameID     *string         `json:"gameId"`
	Properties json.RawMessage `json:"properties"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type adminPage struct {
	Items      []adminEvent `json:"items"`
	NextCursor *string      `json:"nextCursor"`
}

func (runtime *testRuntime) loginAdmin() {
	runtime.request(runtime.admin, http.MethodPost, "/api/v1/admin/auth/login", map[string]any{
		"username": os.Getenv("ANALYTICS_E2E_ADMIN_USERNAME"),
		"password": os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD"),
	}, map[string]string{"Origin": runtime.origin}, http.StatusOK)
}

func (runtime *testRuntime) exerciseAdminPaginationAndPrivacy(startedAt time.Time, gameID, shareSecret string) {
	runtime.loginAdmin()
	if _, err := runtime.db.Exec(`
		UPDATE behavior_events SET created_at = UTC_TIMESTAMP(6)
		WHERE event_name = 'creator.page_viewed'`); err != nil {
		runtime.t.Fatalf("create equal-timestamp pagination fixture: %v", err)
	}
	seen := map[string]struct{}{}
	path := "/api/v1/admin/behavior-events?limit=2"
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		raw := runtime.request(runtime.admin, http.MethodGet, path, nil, nil, http.StatusOK)
		if bytes.Contains(raw, []byte("clientEventId")) {
			runtime.t.Fatal("admin API exposed clientEventId")
		}
		for _, canary := range runtime.privacyCanaries(shareSecret, true) {
			if canary != "" && bytes.Contains(raw, []byte(canary)) {
				runtime.t.Fatal("sensitive canary appeared in admin behavior response")
			}
		}
		page := decode[adminPage](runtime.t, raw)
		for _, event := range page.Items {
			if _, duplicate := seen[event.ID]; duplicate {
				runtime.t.Fatalf("admin cursor pagination repeated event %s", event.ID)
			}
			seen[event.ID] = struct{}{}
			if event.LoginID != nil && *event.LoginID != runtime.loginID {
				runtime.t.Fatal("admin loginId was not derived from the expected user join")
			}
			if bytes.Contains(event.Properties, []byte(runtime.loginID)) {
				runtime.t.Fatal("loginId leaked into event properties")
			}
		}
		if page.NextCursor == nil {
			break
		}
		path = "/api/v1/admin/behavior-events?limit=2&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	var total int
	if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM behavior_events`).Scan(&total); err != nil {
		runtime.t.Fatalf("count behavior events: %v", err)
	}
	if len(seen) != total {
		runtime.t.Fatalf("admin cursor pagination returned %d of %d events", len(seen), total)
	}

	to := time.Now().UTC().Add(time.Second)
	filteredPath := "/api/v1/admin/behavior-events?limit=100&gameId=" + url.QueryEscape(gameID) +
		"&from=" + url.QueryEscape(startedAt.Format(time.RFC3339Nano)) +
		"&to=" + url.QueryEscape(to.Format(time.RFC3339Nano))
	filtered := decode[adminPage](runtime.t, runtime.request(runtime.admin, http.MethodGet, filteredPath, nil, nil, http.StatusOK))
	if len(filtered.Items) == 0 {
		runtime.t.Fatal("admin time/game filter returned no matching events")
	}
	for _, event := range filtered.Items {
		if event.GameID == nil || *event.GameID != gameID || event.CreatedAt.Before(startedAt) || !event.CreatedAt.Before(to) {
			runtime.t.Fatal("admin time/game filter crossed an exclusive boundary")
		}
	}

	var minimum, maximum time.Time
	if err := runtime.db.QueryRow(`SELECT MIN(created_at), MAX(created_at) FROM behavior_events`).Scan(&minimum, &maximum); err != nil {
		runtime.t.Fatalf("read behavior time boundaries: %v", err)
	}
	excluded := decode[adminPage](runtime.t, runtime.request(runtime.admin, http.MethodGet,
		"/api/v1/admin/behavior-events?limit=100&to="+url.QueryEscape(minimum.UTC().Format(time.RFC3339Nano)),
		nil, nil, http.StatusOK))
	if len(excluded.Items) != 0 {
		runtime.t.Fatal("exclusive admin to boundary included an equal timestamp")
	}
	included := decode[adminPage](runtime.t, runtime.request(runtime.admin, http.MethodGet,
		"/api/v1/admin/behavior-events?limit=100&from="+url.QueryEscape(maximum.UTC().Format(time.RFC3339Nano)),
		nil, nil, http.StatusOK))
	if len(included.Items) == 0 {
		runtime.t.Fatal("inclusive admin from boundary excluded an equal timestamp")
	}
}

func (runtime *testRuntime) exerciseInvalidSharesAndDeletion(gameID string, active shareFixture) {
	expired := runtime.createShare(gameID)
	expiredClient := runtime.openPlaySession(expired)
	if _, err := runtime.db.Exec(`
		UPDATE share_links
		SET created_at = UTC_TIMESTAMP(6) - INTERVAL 2 DAY,
		    expires_at = UTC_TIMESTAMP(6) - INTERVAL 1 DAY
		WHERE id = ?`, expired.id); err != nil {
		runtime.t.Fatalf("expire isolated share fixture: %v", err)
	}
	runtime.assertPlayEventRejected(expiredClient, "expired")

	runtime.request(runtime.creator, http.MethodDelete, "/api/v1/games/"+gameID+"/share-links/"+active.id,
		nil, runtime.creatorHeaders(), http.StatusOK)
	runtime.assertPlayEventRejected(runtime.play, "revoked")

	deletionShare := runtime.createShare(gameID)
	deletionClient := runtime.openPlaySession(deletionShare)
	rows, err := runtime.db.Query(`SELECT id FROM games ORDER BY id`)
	if err != nil {
		runtime.t.Fatalf("list isolated games for deletion: %v", err)
	}
	var gameIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			runtime.t.Fatalf("scan isolated game: %v", err)
		}
		gameIDs = append(gameIDs, id)
	}
	if err := rows.Close(); err != nil {
		runtime.t.Fatalf("close isolated game rows: %v", err)
	}
	for _, id := range gameIDs {
		runtime.request(runtime.creator, http.MethodDelete, "/api/v1/games/"+id, nil,
			runtime.creatorHeaders(), http.StatusAccepted)
	}
	runtime.assertPlayEventRejected(deletionClient, "deleting")

	logs := runtime.runWorker(false, func() {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&count); err != nil {
				runtime.t.Fatalf("count games pending deletion: %v", err)
			}
			if count == 0 {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		runtime.t.Fatal("isolated game deletion jobs did not complete")
	})
	runtime.assertLogCanaries(logs.String(), deletionShare.secret)
	var snapshotCount int
	if err := runtime.db.QueryRow(`SELECT COUNT(*) FROM behavior_events WHERE game_id = ?`, gameID).Scan(&snapshotCount); err != nil {
		runtime.t.Fatalf("count behavior snapshots after deletion: %v", err)
	}
	if snapshotCount == 0 {
		runtime.t.Fatal("game deletion removed behavior snapshots")
	}
}

func (runtime *testRuntime) openPlaySession(share shareFixture) *http.Client {
	runtime.t.Helper()
	client := clientWithJar(runtime.t)
	headers := map[string]string{"Origin": runtime.origin}
	payload := map[string]any{"secret": share.secret}
	resolve := runtime.request(client, http.MethodPost, "/api/v1/public/shares/"+share.publicID+"/resolve",
		payload, headers, http.StatusOK)
	runtime.assertResponsePrivate(resolve, share.secret, false)
	session := runtime.request(client, http.MethodPost, "/api/v1/public/shares/"+share.publicID+"/play-sessions",
		payload, headers, http.StatusCreated)
	runtime.assertResponsePrivate(session, share.secret, false)
	return client
}

func (runtime *testRuntime) assertPlayEventRejected(client *http.Client, state string) {
	runtime.t.Helper()
	before := runtime.behaviorCount()
	runtime.request(client, http.MethodPost, "/api/v1/public/play-sessions/current/events", map[string]any{
		"eventName": "play.replayed", "clientEventId": uuidV4(runtime.t), "properties": map[string]any{"mode": "public"},
	}, map[string]string{"Origin": runtime.origin}, http.StatusUnauthorized)
	if after := runtime.behaviorCount(); after != before {
		runtime.t.Fatalf("%s play session rejection changed behavior count", state)
	}
}

func (runtime *testRuntime) exerciseAnalyticsOutage() {
	if _, err := runtime.db.Exec(`RENAME TABLE behavior_events TO behavior_events_e2e_hold`); err != nil {
		runtime.t.Fatalf("temporarily isolate analytics table: %v", err)
	}
	restored := false
	defer func() {
		if !restored {
			_, _ = runtime.db.Exec(`RENAME TABLE behavior_events_e2e_hold TO behavior_events`)
		}
	}()
	runtime.request(runtime.creator, http.MethodPost, "/api/v1/games", map[string]any{
		"title": "Analytics unavailable", "description": "business must continue",
		"templateId": "love-journey", "templateVersion": "1.1.0",
		"sceneInputs": map[string]any{
			"loveLetter": runtime.memoryText, "letterPassword": "2580", "passwordHint": "four digits",
		},
	}, runtime.creatorHeaders(), http.StatusCreated)
	if _, err := runtime.db.Exec(`RENAME TABLE behavior_events_e2e_hold TO behavior_events`); err != nil {
		runtime.t.Fatalf("restore analytics table: %v", err)
	}
	restored = true
	if !strings.Contains(runtime.apiLogs.String(), "ANALYTICS_WRITE_FAILED") {
		runtime.t.Fatal("analytics outage did not produce the sanitized warning code")
	}
}

func (runtime *testRuntime) exerciseSurfaceMatrix() {
	type surfaceExpectation struct {
		surface       string
		creatorStatus int
		publicStatus  int
	}
	for _, test := range []surfaceExpectation{
		{surface: "app", creatorStatus: http.StatusUnauthorized, publicStatus: http.StatusNotFound},
		{surface: "play", creatorStatus: http.StatusNotFound, publicStatus: http.StatusUnauthorized},
		{surface: "all", creatorStatus: http.StatusUnauthorized, publicStatus: http.StatusUnauthorized},
	} {
		apiPort := freePort(runtime.t)
		baseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort)
		process := startProcess(runtime.t, os.Getenv("ANALYTICS_E2E_API_BIN"),
			processEnvironment(test.surface, apiPort, freePort(runtime.t), false))
		waitReady(runtime.t, baseURL+"/health/ready", process)
		client := clientWithJar(runtime.t)
		creatorRequest, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/auth/session", nil)
		creatorResponse, err := client.Do(creatorRequest)
		if err != nil {
			process.stop(runtime.t)
			runtime.t.Fatalf("request %s creator surface: %v", test.surface, err)
		}
		_ = creatorResponse.Body.Close()
		publicRequest, _ := http.NewRequest(http.MethodPost,
			baseURL+"/api/v1/public/play-sessions/current/events", strings.NewReader(`{}`))
		publicRequest.Header.Set("Origin", runtime.origin)
		publicRequest.Header.Set("Content-Type", "application/json")
		publicResponse, err := client.Do(publicRequest)
		if err != nil {
			process.stop(runtime.t)
			runtime.t.Fatalf("request %s public surface: %v", test.surface, err)
		}
		_ = publicResponse.Body.Close()
		process.stop(runtime.t)
		if creatorResponse.StatusCode != test.creatorStatus || publicResponse.StatusCode != test.publicStatus {
			runtime.t.Fatalf("surface %s matrix=%d/%d, want %d/%d", test.surface,
				creatorResponse.StatusCode, publicResponse.StatusCode, test.creatorStatus, test.publicStatus)
		}
		runtime.assertLogCanaries(process.logs.String(), "")
	}
}

func (runtime *testRuntime) assertCompleteEventSet() {
	wanted := []string{
		"creator.registered", "creator.logged_in", "creator.page_viewed", "game.created",
		"game.version_created", "asset.uploaded", "generation.submitted", "generation.succeeded",
		"generation.failed", "share.created", "share.opened", "play.started", "play.completed", "play.replayed",
	}
	rows, err := runtime.db.Query(`SELECT event_name, COUNT(*) FROM behavior_events GROUP BY event_name`)
	if err != nil {
		runtime.t.Fatalf("query complete event set: %v", err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			rows.Close()
			runtime.t.Fatalf("scan complete event set: %v", err)
		}
		counts[name] = count
	}
	if err := rows.Close(); err != nil {
		runtime.t.Fatalf("close complete event set: %v", err)
	}
	for _, name := range wanted {
		if counts[name] < 1 {
			runtime.t.Errorf("required behavior event %s was not recorded", name)
		}
	}
}

func (runtime *testRuntime) assertSnapshotsAndPrivacy(gameID, versionID string) {
	var linked int
	if err := runtime.db.QueryRow(`
		SELECT COUNT(*) FROM behavior_events
		WHERE game_id = ? AND game_version_id = ? AND user_id = ?`,
		gameID, versionID, runtime.creatorID).Scan(&linked); err != nil {
		runtime.t.Fatalf("verify trusted behavior links: %v", err)
	}
	if linked == 0 {
		runtime.t.Fatal("trusted behavior links were not preserved as snapshots")
	}

	rows, err := runtime.db.Query(`
		SELECT event_name, CAST(properties AS CHAR), COALESCE(client_event_id, ''),
		       COALESCE(request_id, ''), COALESCE(user_session_id, '')
		FROM behavior_events`)
	if err != nil {
		runtime.t.Fatalf("query behavior privacy audit: %v", err)
	}
	for rows.Next() {
		var name, properties, clientEventID, requestID, sessionID string
		if err := rows.Scan(&name, &properties, &clientEventID, &requestID, &sessionID); err != nil {
			rows.Close()
			runtime.t.Fatalf("scan behavior privacy audit: %v", err)
		}
		combined := name + properties + clientEventID + requestID + sessionID
		for _, canary := range runtime.privacyCanaries("", false) {
			if canary != "" && strings.Contains(combined, canary) {
				rows.Close()
				runtime.t.Fatal("sensitive canary appeared in behavior event storage")
			}
		}
	}
	if err := rows.Close(); err != nil {
		runtime.t.Fatalf("close behavior privacy audit: %v", err)
	}
}

func (runtime *testRuntime) assertLogsPrivate(secret string) {
	runtime.assertLogCanaries(runtime.apiLogs.String(), secret)
	runtime.assertLogCanaries(runtime.viteLogs.String(), secret)
}

func (runtime *testRuntime) assertLogCanaries(logs, secret string) {
	runtime.t.Helper()
	for _, canary := range runtime.privacyCanaries(secret, false) {
		if canary != "" && strings.Contains(logs, canary) {
			runtime.t.Fatal("sensitive canary appeared in application logs")
		}
	}
}

func (runtime *testRuntime) assertResponsePrivate(raw []byte, secret string, allowAdminLoginID bool) {
	runtime.t.Helper()
	for _, canary := range runtime.privacyCanaries(secret, allowAdminLoginID) {
		if canary != "" && bytes.Contains(raw, []byte(canary)) {
			runtime.t.Fatal("sensitive canary appeared in an analytics-related API response")
		}
	}
}

func (runtime *testRuntime) privacyCanaries(secret string, allowAdminLoginID bool) []string {
	canaries := []string{
		runtime.memoryText, runtime.password, runtime.creatorPasswordHash, runtime.fileName, runtime.imageBase64, secret,
		os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD"), os.Getenv("ANALYTICS_E2E_ADMIN_PASSWORD_HASH"), runtime.csrf,
	}
	canaries = append(canaries, runtime.artifactCanaries...)
	canaries = append(canaries, runtime.shareSecrets...)
	canaries = append(canaries, runtime.invitationCodes...)
	if !allowAdminLoginID {
		canaries = append(canaries, runtime.loginID)
	}
	for _, rawURL := range []string{runtime.baseURL, runtime.origin} {
		base, err := url.Parse(rawURL)
		if err == nil {
			for _, client := range []*http.Client{runtime.creator, runtime.play, runtime.admin} {
				for _, cookie := range client.Jar.Cookies(base) {
					canaries = append(canaries, cookie.Value)
				}
			}
		}
	}
	return canaries
}

func uuidV4(t *testing.T) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("create UUID fixture: %v", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func randomHex(t *testing.T, bytesCount int) string {
	t.Helper()
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("create random E2E fixture: %v", err)
	}
	return fmt.Sprintf("%x", value)
}
