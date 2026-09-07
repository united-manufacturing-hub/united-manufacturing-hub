// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration_test

// mcStack boots the REAL ManagementConsole backend + router (built from source
// pulled by `make pull-managementconsole`) and drives a real umh-core container
// through them end-to-end, exactly the way ManagementConsole's own Playwright
// e2e suite runs them: backend and router as local Go processes, Postgres and
// Redis as throwaway Docker containers.
//
//	                 test process (the "user")
//	          POST /api/v2/user/push  |  GET /api/v2/user/pull
//	                                  v
//	  umh-core container ──▶ router (:R) ──/api──▶ backend (:B) ──▶ Postgres + Redis
//	   login/pull/push        strips /api          v2/instance/*, v2/user/*
//
// The backend is a passthrough relay for the message Content, so umh-core keeps
// using its own corev1 codec (base64(JSON), no encryption) — the same wire the
// old fakeBackend used. What is now REAL: instance login against a DB row,
// JWT-scoped user auth, and the v3 Redis message queue routing.
//
// mcStack mirrors the method surface of the old fakeBackend
// (apiURL/loginSeen/enqueueEditProtocolConverter/terminalReplyState/replyDump)
// so the staleness spec is unchanged apart from the constructor.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	// mcAuthToken is the raw auth token; it MUST match the AuthToken set in
	// buildStalenessConfig. umh-core sends LoginHash(mcAuthToken) as the Bearer
	// token, so that hash is what we seed into instances.auth_token.
	mcAuthToken = "test-token"

	// mcUserEmail is the seeded user's email. The edit action carries it, the
	// agent echoes it on its reply, and the backend uses it to route the reply
	// to this user's queue (mq:v3:live:user:<userID>).
	mcUserEmail = "harness-user@umh.test"

	mcPGImage    = "postgres:15.4"
	mcRedisImage = "redis:7-alpine"
	mcDBName     = "management-console"
	mcRedisPass  = "management-console"
)

// mcBuild* memoize the one-time build of the backend + router binaries so both
// staleness specs (fsmv1, fsmv2) reuse them instead of rebuilding per spec.
var (
	mcBuildOnce  sync.Once
	mcBackendBin string
	mcRouterBin  string
	mcBuildErr   error
)

type mcStack struct {
	jwtSecret      string
	instanceUUID   uuid.UUID
	userEmail      string
	userID         int64
	sessionVersion string
	userCookie     string // minted USER_SCOPE JWT, sent as the `token` cookie

	pgPort      int
	redisPort   int
	backendPort int
	routerPort  int

	pgContainer    string
	redisContainer string

	backendProc *exec.Cmd
	routerProc  *exec.Cmd

	mu        sync.Mutex
	replies   map[uuid.UUID]models.ActionReplyState
	replyMsgs map[uuid.UUID][]string

	stopPoll chan struct{}
	pollDone chan struct{}

	logDir string
}

// mcDir returns the directory holding the pulled ManagementConsole source
// (backend/, router/, shared/, cryptolib/, frontend/static/requirements.json).
// Defaults to ./managementconsole next to the integration package; overridable
// with MC_DIR (the Makefile passes it).
func mcDir() (string, error) {
	if d := os.Getenv("MC_DIR"); d != "" {
		return d, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return filepath.Join(wd, "managementconsole"), nil
}

// mcFreePort grabs an ephemeral TCP port and releases it. There is a small race
// between release and reuse, acceptable for a serial integration test.
func mcFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	defer func() { _ = ln.Close() }()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

// buildMCBinaries builds the backend and router binaries once from the pulled
// source. It fails with an actionable message if the source is missing.
func buildMCBinaries() (string, string, error) {
	mcBuildOnce.Do(func() {
		dir, err := mcDir()
		if err != nil {
			mcBuildErr = err

			return
		}

		backendDir := filepath.Join(dir, "backend")
		routerDir := filepath.Join(dir, "router")

		if _, err := os.Stat(backendDir); err != nil {
			mcBuildErr = fmt.Errorf("ManagementConsole source not found at %s (run `make pull-managementconsole`): %w", dir, err)

			return
		}

		// The backend build embeds frontend/static/requirements.json into the
		// demo simulator; copy it in first (mirrors `make copy_requirements_json`).
		reqSrc := filepath.Join(dir, "frontend", "static", "requirements.json")

		reqDst := filepath.Join(backendDir, "cmd", "demo_simulator_v3", "requirements.json")
		if data, err := os.ReadFile(reqSrc); err == nil {
			_ = os.WriteFile(reqDst, data, 0o644)
		}

		binDir, err := os.MkdirTemp("", "mc-bins-")
		if err != nil {
			mcBuildErr = err

			return
		}

		mcBackendBin = filepath.Join(binDir, "mc-backend")
		mcRouterBin = filepath.Join(binDir, "mc-router")

		for _, b := range []struct {
			out, srcDir string
		}{
			{mcBackendBin, backendDir},
			{mcRouterBin, routerDir},
		} {
			// -mod=mod: the pulled tree ships no vendor/ dir, so resolve modules
			// against the module cache / GOPROXY.
			cmd := exec.Command("go", "build", "-mod=mod", "-o", b.out, "./cmd")
			cmd.Dir = b.srcDir

			cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
			if out, err := cmd.CombinedOutput(); err != nil {
				mcBuildErr = fmt.Errorf("go build in %s failed: %w\n%s", b.srcDir, err, out)

				return
			}
		}
	})

	return mcBackendBin, mcRouterBin, mcBuildErr
}

// newMCStack boots Postgres, Redis, the backend and the router, seeds a
// loginable instance, and starts a background poller that drains the user
// queue. The caller must call stop() when done.
func newMCStack(ctx context.Context) (*mcStack, error) {
	backendBin, routerBin, err := buildMCBinaries()
	if err != nil {
		return nil, err
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, err
	}

	s := &mcStack{
		jwtSecret:    hex.EncodeToString(secretBytes),
		instanceUUID: uuid.New(),
		userEmail:    mcUserEmail,
		replies:      make(map[uuid.UUID]models.ActionReplyState),
		replyMsgs:    make(map[uuid.UUID][]string),
		stopPoll:     make(chan struct{}),
		pollDone:     make(chan struct{}),
	}

	for _, p := range []*int{&s.pgPort, &s.redisPort, &s.backendPort, &s.routerPort} {
		port, err := mcFreePort()
		if err != nil {
			return nil, err
		}

		*p = port
	}

	s.logDir, err = os.MkdirTemp("", "mc-logs-")
	if err != nil {
		return nil, err
	}

	suffix := uuid.New().String()[:8]
	s.pgContainer = "mc-pg-" + suffix
	s.redisContainer = "mc-redis-" + suffix

	// Postgres + Redis containers.
	if _, err := runDockerCommand("run", "-d", "--name", s.pgContainer,
		"-e", "POSTGRES_PASSWORD=password", "-e", "POSTGRES_DB="+mcDBName,
		"-p", fmt.Sprintf("%d:5432", s.pgPort), mcPGImage); err != nil {
		return nil, fmt.Errorf("start postgres: %w", err)
	}

	if _, err := runDockerCommand("run", "-d", "--name", s.redisContainer,
		"-p", fmt.Sprintf("%d:6379", s.redisPort), mcRedisImage,
		"redis-server", "--requirepass", mcRedisPass); err != nil {
		return nil, fmt.Errorf("start redis: %w", err)
	}

	if err := s.waitForPostgres(ctx); err != nil {
		return nil, err
	}

	// Backend (migrates the schema on boot).
	if err := s.startBackend(backendBin); err != nil {
		return nil, err
	}

	if err := mcWaitHTTP(fmt.Sprintf("http://127.0.0.1:%d/v2/health", s.backendPort), 90*time.Second); err != nil {
		return nil, fmt.Errorf("backend never became healthy: %w", err)
	}

	// Seed one company + user + instance (all in the same company so the user is
	// authorized to push to the instance and the reply routes back).
	if err := s.seed(ctx); err != nil {
		return nil, err
	}

	s.userCookie, err = mintUserJWT(s.jwtSecret, s.userEmail, s.userID, s.sessionVersion)
	if err != nil {
		return nil, err
	}

	// Router (thin reverse proxy; boots on placeholder R2 creds).
	if err := s.startRouter(routerBin); err != nil {
		return nil, err
	}

	if err := mcWaitHTTP(fmt.Sprintf("http://127.0.0.1:%d/api/v2/health", s.routerPort), 60*time.Second); err != nil {
		return nil, fmt.Errorf("router never became healthy: %w", err)
	}

	go s.pollUserQueue()

	return s, nil
}

func (s *mcStack) dbURL() string {
	return fmt.Sprintf("host=127.0.0.1 port=%d user=postgres password=password dbname=%s sslmode=disable", s.pgPort, mcDBName)
}

func (s *mcStack) pgxURL() string {
	return fmt.Sprintf("postgres://postgres:password@127.0.0.1:%d/%s?sslmode=disable", s.pgPort, mcDBName)
}

func (s *mcStack) redisURL() string {
	return fmt.Sprintf("redis://default:%s@127.0.0.1:%d/0", mcRedisPass, s.redisPort)
}

func (s *mcStack) waitForPostgres(ctx context.Context) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		connCtx, cancel := context.WithTimeout(ctx, 3*time.Second)

		conn, err := pgx.Connect(connCtx, s.pgxURL())
		if err == nil {
			_, pingErr := conn.Exec(connCtx, "SELECT 1")
			_ = conn.Close(connCtx)

			cancel()

			if pingErr == nil {
				return nil
			}
		} else {
			cancel()
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("postgres on port %d not ready in time", s.pgPort)
}

func (s *mcStack) startBackend(bin string) error {
	dir, _ := mcDir()

	cmd := exec.Command(bin)
	cmd.Dir = filepath.Join(dir, "backend")

	cmd.Env = append(os.Environ(),
		"TESTING=true",
		"LOGGING_LEVEL=DEBUG",
		fmt.Sprintf("PORT=%d", s.backendPort),
		"JWT_SECRET_KEY="+s.jwtSecret,
		"DATABASE_URL="+s.dbURL(),
		"REDIS_URL="+s.redisURL(),
		// Auth0 is only used by the user-login routes we never touch; dummy values
		// are enough for the instance + user push/pull paths.
		"AUTH0_DOMAIN=dev.example.com",
		"AUTH0_CLIENT_ID=dummy",
	)

	logFile, err := os.Create(filepath.Join(s.logDir, "backend.log"))
	if err != nil {
		return err
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start backend: %w", err)
	}

	s.backendProc = cmd

	return nil
}

func (s *mcStack) startRouter(bin string) error {
	dir, _ := mcDir()

	cmd := exec.Command(bin)
	cmd.Dir = filepath.Join(dir, "router")

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", s.routerPort),
		fmt.Sprintf("CUSTOM_API_URL=http://127.0.0.1:%d", s.backendPort),
		"CUSTOM_FRONTEND_URL=http://127.0.0.1:1420",
		"LOGGING_LEVEL=DEVELOPMENT",
		// r2.SetupR2 only constructs S3 clients (no boot-time network call), so
		// placeholder credentials let the router boot. The /api forward path never
		// touches R2.
		"R2_ACCOUNT_ID=placeholder",
		"R2_ACCESS_KEY_ID_OCI=placeholder",
		"R2_ACCESS_KEY_SECRET_OCI=placeholder",
		"R2_ACCESS_KEY_ID_BINARIES=placeholder",
		"R2_ACCESS_KEY_SECRET_BINARIES=placeholder",
		"R2_ACCESS_KEY_ID_STATIC_BINARIES=placeholder",
		"R2_ACCESS_KEY_SECRET_STATIC_BINARIES=placeholder",
	)

	logFile, err := os.Create(filepath.Join(s.logDir, "router.log"))
	if err != nil {
		return err
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start router: %w", err)
	}

	s.routerProc = cmd

	return nil
}

// seed inserts the minimal rows for a successful instance login and an
// authorized user push: one company, one user (same company), one instance
// (same company) whose auth_token is LoginHash(mcAuthToken).
func (s *mcStack) seed(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, s.pgxURL())
	if err != nil {
		return fmt.Errorf("seed connect: %w", err)
	}

	defer func() { _ = conn.Close(ctx) }()

	var companyID int64
	if err := conn.QueryRow(ctx,
		`INSERT INTO companies (created_at, updated_at, uuid, name)
		 VALUES (now(), now(), $1, $2) RETURNING id`,
		uuid.New().String(), "HarnessCo",
	).Scan(&companyID); err != nil {
		return fmt.Errorf("seed company: %w", err)
	}

	// session_version must be set and match the minted token: GET /v2/user/pull
	// resolves the user by numeric id and rejects the token if the row's
	// session_version is nil or differs (auth.go ValidateTokenMiddleware).
	s.sessionVersion = uuid.New().String()
	if err := conn.QueryRow(ctx,
		`INSERT INTO users (created_at, updated_at, email, password, company_id, session_version)
		 VALUES (now(), now(), $1, $2, $3, $4) RETURNING id`,
		s.userEmail, "seeded-no-login", companyID, s.sessionVersion,
	).Scan(&s.userID); err != nil {
		return fmt.Errorf("seed user: %w", err)
	}

	// umh-core sends Bearer LoginHash(token) = Sha3(Sha3(token)); the backend
	// compares that literal value against instances.auth_token.
	authTokenHash := hash.Sha3Hash(hash.Sha3Hash(mcAuthToken))
	if _, err := conn.Exec(ctx,
		`INSERT INTO instances (created_at, updated_at, uuid, auth_token, name, verified, company_id)
		 VALUES (now(), now(), $1, $2, $3, false, $4)`,
		s.instanceUUID.String(), authTokenHash, "HarnessInstance", companyID,
	); err != nil {
		return fmt.Errorf("seed instance: %w", err)
	}

	return nil
}

// mintUserJWT hand-signs a HS256 USER_SCOPE token carrying the numeric user_id
// and the row's session_version. POST /v2/user/push resolves by email, but GET
// /v2/user/pull resolves by numeric id and enforces the session_version match,
// so both must be present (auth.go ValidateTokenMiddleware).
func mintUserJWT(secret, email string, userID int64, sessionVersion string) (string, error) {
	now := time.Now()

	encode := func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}

		return base64.RawURLEncoding.EncodeToString(b), nil
	}

	header, err := encode(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}

	claims, err := encode(map[string]any{
		"scope":              "user",
		"email":              email,
		"user_id":            userID,
		"session_version":    sessionVersion,
		"original_issued_at": now.Unix(),
		"iat":                now.Unix(),
		"exp":                now.Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}

	signingInput := header + "." + claims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

// apiURL is the base URL umh-core (inside the container) uses to reach the
// router. The path already includes /api so the router forwards to the backend
// (umh-core appends /v2/instance/... to this base).
func (s *mcStack) apiURL() string {
	return fmt.Sprintf("http://host.docker.internal:%d/api", s.routerPort)
}

// hostRouterURL is the base URL the TEST process (on the host) uses to reach the
// router.
func (s *mcStack) hostRouterURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/api", s.routerPort)
}

// enqueueEditProtocolConverter sends an edit-protocol-converter action to the
// instance by POSTing it to the real backend's user-push endpoint (through the
// router), authenticated with the minted user cookie.
// If readDFC is non-nil it is attached under the "readDFC" key so the edited
// bridge keeps a non-empty DFC type (connection-gated rollout); a nil readDFC
// produces a connection-only edit (DFCType empty).
func (s *mcStack) enqueueEditProtocolConverter(actionUUID, pcUUID uuid.UUID, pcName, ip string, port uint32, readDFC map[string]any) error {
	actionPayload := map[string]any{
		"uuid": pcUUID.String(),
		"name": pcName,
		"connection": map[string]any{
			"ip":   ip,
			"port": port,
		},
		"location": map[string]any{
			"0": "test-enterprise",
		},
	}

	if readDFC != nil {
		actionPayload["readDFC"] = readDFC
	}

	messageContent := models.UMHMessageContent{
		MessageType: models.Action,
		Payload: models.ActionMessagePayload{
			ActionType:    models.EditProtocolConverter,
			ActionUUID:    actionUUID,
			ActionPayload: actionPayload,
		},
	}

	encoded, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		return fmt.Errorf("encode action: %w", err)
	}

	payload := backend_api_structs.PushPayload{
		UMHMessages: []models.UMHMessage{
			{
				Email:        s.userEmail,
				InstanceUUID: s.instanceUUID,
				Content:      encoded,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, s.hostRouterURL()+"/v2/user/push", strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "token", Value: s.userCookie})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("user push: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("user push returned status %d", resp.StatusCode)
	}

	return nil
}

// pollUserQueue continuously drains GET /v2/user/pull (which RPOPs the user
// queue) and records every action-reply, since replies arrive over time and the
// pull consumes them.
func (s *mcStack) pollUserQueue() {
	defer close(s.pollDone)

	client := &http.Client{Timeout: 30 * time.Second}

	for {
		select {
		case <-s.stopPoll:
			return
		default:
		}

		req, err := http.NewRequest(http.MethodGet, s.hostRouterURL()+"/v2/user/pull", nil)
		if err != nil {
			time.Sleep(500 * time.Millisecond)

			continue
		}

		req.AddCookie(&http.Cookie{Name: "token", Value: s.userCookie})
		req.Header.Set("X-Features", "longpoll")

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(500 * time.Millisecond)

			continue
		}

		if resp.StatusCode == http.StatusOK {
			var payload backend_api_structs.PullPayload
			if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
				for _, msg := range payload.UMHMessages {
					s.recordReply(msg)
				}
			}
		}

		_ = resp.Body.Close()
	}
}

// recordReply decodes one pulled message and, if it is an action-reply, records
// its state keyed by the action UUID.
func (s *mcStack) recordReply(msg models.UMHMessage) {
	if msg.Content == "" {
		return
	}

	content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
	if err != nil {
		return
	}

	if content.MessageType != models.ActionReply {
		return
	}

	raw, err := json.Marshal(content.Payload)
	if err != nil {
		return
	}

	var reply models.ActionReplyMessagePayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return
	}

	s.mu.Lock()
	s.replies[reply.ActionUUID] = reply.ActionReplyState
	s.replyMsgs[reply.ActionUUID] = append(s.replyMsgs[reply.ActionUUID],
		fmt.Sprintf("%s: %v", reply.ActionReplyState, reply.ActionReplyPayload))
	s.mu.Unlock()
}

// replyDump returns the ordered "state: message" history captured for the action.
func (s *mcStack) replyDump(actionUUID uuid.UUID) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.replyMsgs[actionUUID]))
	copy(out, s.replyMsgs[actionUUID])

	return out
}

// terminalReplyState returns the captured reply state for the action and whether
// it is terminal (action-success or action-failure).
func (s *mcStack) terminalReplyState(actionUUID uuid.UUID) (models.ActionReplyState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.replies[actionUUID]
	if !ok {
		return "", false
	}

	terminal := state == models.ActionFinishedSuccessfull || state == models.ActionFinishedWithFailure

	return state, terminal
}

// loginSeen reports whether the container has completed at least one instance
// login. The backend flips instances.verified to true on first login
// (instance_login.go), so we poll that flag.
func (s *mcStack) loginSeen() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, s.pgxURL())
	if err != nil {
		return false
	}

	defer func() { _ = conn.Close(ctx) }()

	var verified bool
	if err := conn.QueryRow(ctx,
		`SELECT verified FROM instances WHERE uuid = $1`, s.instanceUUID.String(),
	).Scan(&verified); err != nil {
		return false
	}

	return verified
}

// stop tears down the poller, the two processes and the two containers, and
// surfaces the backend/router logs on failure.
func (s *mcStack) stop() {
	if s.stopPoll != nil {
		close(s.stopPoll)

		select {
		case <-s.pollDone:
		case <-time.After(5 * time.Second):
		}
	}

	if CurrentSpecReport().Failed() {
		s.dumpLog("backend.log")
		s.dumpLog("router.log")
	}

	for _, p := range []*exec.Cmd{s.routerProc, s.backendProc} {
		if p != nil && p.Process != nil {
			_ = p.Process.Kill()
			_, _ = p.Process.Wait()
		}
	}

	for _, name := range []string{s.pgContainer, s.redisContainer} {
		if name != "" {
			_, _ = runDockerCommand("rm", "-f", name)
		}
	}

	if s.logDir != "" {
		_ = os.RemoveAll(s.logDir)
	}
}

func (s *mcStack) dumpLog(name string) {
	data, err := os.ReadFile(filepath.Join(s.logDir, name))
	if err != nil {
		return
	}

	GinkgoWriter.Printf("\n===== ManagementConsole %s =====\n%s\n===== end %s =====\n", name, string(data), name)
}

// encodingChooseCorev1 selects the corev1 encoder for this (test) process so it
// matches the encoder the container selects in cmd/main.go. Messages are
// base64(JSON) with no encryption. The backend relays Content opaquely, so this
// codec governs both the action we send and the replies we decode.
func encodingChooseCorev1() {
	encoding.ChooseEncoder(encoding.EncodingCorev1)
}

// mcWaitHTTP polls url until it returns any HTTP response (2xx/4xx both mean the
// server is up) or the timeout elapses.
func mcWaitHTTP(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("%s not reachable within %s", url, timeout)
}
