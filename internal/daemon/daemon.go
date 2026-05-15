package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyonbrown4d/bu1ld/internal/app"
	"github.com/lyonbrown4d/bu1ld/internal/config"

	"github.com/samber/oops"
)

const (
	stateFileName = "daemon.json"
	startTimeout  = 5 * time.Second
	stopTimeout   = 5 * time.Second
)

var controlClient = &http.Client{Timeout: 2 * time.Second}

type ExecuteFunc func(context.Context, config.Config, io.Writer, app.CommandRequest) error

type State struct {
	WorkDir   string    `json:"work_dir"`
	Endpoint  string    `json:"endpoint"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

type Status struct {
	Running bool
	Stale   bool
	State   State
	Message string
}

type commandEnvelope struct {
	Config  config.Config      `json:"config"`
	Request app.CommandRequest `json:"request"`
}

type commandResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func StatePath(cfg config.Config) string {
	return filepath.Join(cfg.StateDir(), stateFileName)
}

func Serve(ctx context.Context, cfg config.Config, output io.Writer, execute ExecuteFunc) error {
	if execute == nil {
		return oops.In("bu1ld.daemon").New("daemon execute function is nil")
	}
	if err := os.MkdirAll(cfg.StateDir(), 0o755); err != nil {
		return oops.In("bu1ld.daemon").
			With("state_dir", cfg.StateDir()).
			Wrapf(err, "create daemon state directory")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return oops.In("bu1ld.daemon").Wrapf(err, "listen on local daemon socket")
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state := State{
		WorkDir:   cfg.WorkDir,
		Endpoint:  "http://" + listener.Addr().String(),
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}
	if err := writeState(cfg, state); err != nil {
		return err
	}
	defer removeState(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/daemon/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("/v1/daemon/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, state)
		go cancel()
	})
	mux.HandleFunc("/v1/commands", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var envelope commandEnvelope
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			http.Error(w, "decode command envelope", http.StatusBadRequest)
			return
		}
		if filepath.Clean(envelope.Config.WorkDir) != filepath.Clean(cfg.WorkDir) {
			http.Error(w, "daemon workspace mismatch", http.StatusConflict)
			return
		}
		if !CanProxy(envelope.Request.Kind) {
			http.Error(w, "command cannot be proxied through daemon", http.StatusBadRequest)
			return
		}

		var buf bytes.Buffer
		result := commandResult{}
		if err := execute(r.Context(), envelope.Config, &buf, envelope.Request); err != nil {
			result.Error = err.Error()
		}
		result.Output = buf.String()
		writeJSON(w, http.StatusOK, result)
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if output != nil {
		_, _ = fmt.Fprintf(output, "daemon listening on %s\n", state.Endpoint)
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return oops.In("bu1ld.daemon").
			With("endpoint", state.Endpoint).
			Wrapf(err, "serve daemon")
	}
	return nil
}

func Start(ctx context.Context, cfg config.Config, spawnArgs []string) (State, bool, error) {
	status, err := Check(ctx, cfg)
	if err != nil {
		return State{}, false, err
	}
	if status.Running {
		return status.State, true, nil
	}
	if err := os.MkdirAll(cfg.StateDir(), 0o755); err != nil {
		return State{}, false, oops.In("bu1ld.daemon").
			With("state_dir", cfg.StateDir()).
			Wrapf(err, "create daemon state directory")
	}

	exe, err := os.Executable()
	if err != nil {
		return State{}, false, oops.In("bu1ld.daemon").Wrapf(err, "resolve executable")
	}

	logFile, err := os.OpenFile(filepath.Join(cfg.StateDir(), "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return State{}, false, oops.In("bu1ld.daemon").
			With("state_dir", cfg.StateDir()).
			Wrapf(err, "open daemon log")
	}
	defer logFile.Close()

	args := append(daemonConfigArgs(cfg), spawnArgs...)
	cmd := exec.CommandContext(context.Background(), exe, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return State{}, false, oops.In("bu1ld.daemon").
			With("executable", exe).
			Wrapf(err, "start daemon process")
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return State{}, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		status, err = Check(ctx, cfg)
		if err != nil {
			return State{}, false, err
		}
		if status.Running {
			return status.State, false, nil
		}
	}
	return State{}, false, oops.In("bu1ld.daemon").
		With("log", filepath.Join(cfg.StateDir(), "daemon.log")).
		New("daemon did not become ready before timeout")
}

func Stop(ctx context.Context, cfg config.Config) (State, bool, error) {
	status, err := Check(ctx, cfg)
	if err != nil {
		return State{}, false, err
	}
	if !status.Running {
		if status.Stale {
			removeState(cfg)
		}
		return status.State, false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, status.State.Endpoint+"/v1/daemon/stop", http.NoBody)
	if err != nil {
		return State{}, false, err
	}
	resp, err := controlClient.Do(req)
	if err != nil {
		return State{}, false, oops.In("bu1ld.daemon").
			With("endpoint", status.State.Endpoint).
			Wrapf(err, "request daemon stop")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return State{}, false, oops.In("bu1ld.daemon").
			With("endpoint", status.State.Endpoint).
			With("status", resp.Status).
			New("daemon stop request failed")
	}

	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return State{}, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		next, err := Check(ctx, cfg)
		if err != nil {
			return State{}, false, err
		}
		if !next.Running {
			removeState(cfg)
			return status.State, true, nil
		}
	}
	return State{}, false, oops.In("bu1ld.daemon").New("daemon did not stop before timeout")
}

func Check(ctx context.Context, cfg config.Config) (Status, error) {
	state, err := readState(cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Status{Message: "daemon status: stopped"}, nil
		}
		return Status{}, err
	}
	if strings.TrimSpace(state.Endpoint) == "" {
		return Status{Stale: true, State: state, Message: "daemon status: stopped (stale state)"}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, state.Endpoint+"/v1/daemon/status", http.NoBody)
	if err != nil {
		return Status{}, err
	}
	resp, err := controlClient.Do(req)
	if err != nil {
		return Status{Stale: true, State: state, Message: "daemon status: stopped (stale state)"}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Status{Stale: true, State: state, Message: "daemon status: stopped (stale state)"}, nil
	}
	var live State
	if err := json.NewDecoder(resp.Body).Decode(&live); err != nil {
		return Status{}, err
	}
	return Status{Running: true, State: live, Message: "daemon status: running"}, nil
}

func TryRun(ctx context.Context, cfg config.Config, output io.Writer, request app.CommandRequest) (bool, error) {
	if !CanProxy(request.Kind) {
		return false, nil
	}
	status, err := Check(ctx, cfg)
	if err != nil || !status.Running {
		return false, err
	}

	body, err := json.Marshal(commandEnvelope{Config: cfg, Request: request})
	if err != nil {
		return true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, status.State.Endpoint+"/v1/commands", bytes.NewReader(body))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var result commandResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return true, err
	}
	if output != nil && result.Output != "" {
		if _, err := io.WriteString(output, result.Output); err != nil {
			return true, err
		}
	}
	if result.Error != "" {
		return true, errors.New(result.Error)
	}
	return true, nil
}

func FormatStatus(status Status) string {
	if !status.Running {
		return status.Message
	}
	return fmt.Sprintf(
		"%s\n  pid: %d\n  endpoint: %s\n  workspace: %s\n  started_at: %s",
		status.Message,
		status.State.PID,
		status.State.Endpoint,
		status.State.WorkDir,
		status.State.StartedAt.Format(time.RFC3339),
	)
}

func CanProxy(kind app.CommandKind) bool {
	switch kind {
	case app.CommandBuild,
		app.CommandTest,
		app.CommandDoctor,
		app.CommandGraph,
		app.CommandTasks,
		app.CommandPackages,
		app.CommandPackagesGraph,
		app.CommandAffected:
		return true
	default:
		return false
	}
}

func daemonConfigArgs(cfg config.Config) []string {
	args := []string{
		"--project-dir", cfg.WorkDir,
		"--file", cfg.BuildFile,
		"--cache-dir", cfg.CacheDir,
	}
	if cfg.NoCache {
		args = append(args, "--no-cache")
	}
	if cfg.RemoteCacheURL != "" {
		args = append(args, "--remote-cache-url", cfg.RemoteCacheURL)
	}
	if !cfg.RemoteCachePull {
		args = append(args, "--remote-cache-pull=false")
	}
	if cfg.RemoteCachePush {
		args = append(args, "--remote-cache-push")
	}
	return args
}

func writeState(cfg config.Config, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.StateDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(StatePath(cfg), data, 0o644)
}

func readState(cfg config.Config) (State, error) {
	data, err := os.ReadFile(StatePath(cfg))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func removeState(cfg config.Config) {
	_ = os.Remove(StatePath(cfg))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
