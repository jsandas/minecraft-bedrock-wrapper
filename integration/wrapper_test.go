//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	key              = "integration-secret"
	operationTimeout = 10 * time.Second
)

type process struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	log  string
}

func build(t *testing.T, output, pkg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", output, pkg)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, data)
	}
}

// cleanEnv prevents local runtime settings from changing the test or triggering a download.
func cleanEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CFG_") || name == "MINECRAFT_VERSION" || name == "AUTH_KEY" ||
			name == "EULA_ACCEPT" || name == "PORT" || name == "APP_DIR" || name == "LISTEN_ADDRESS" {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func start(t *testing.T, binary, dir string, env, args []string) *process {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "wrapper.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(cleanEnv(), env...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	// The wrapper does not terminate its child on forced shutdown. Kill the group
	// on cleanup so a failed test cannot leave a fake server behind.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	p := &process{cmd: cmd, done: make(chan struct{}), log: logPath}
	go func() {
		p.err = cmd.Wait()
		logFile.Close()
		close(p.done)
	}()
	t.Cleanup(func() {
		// The wrapper may exit before its child (for example, if HTTP startup
		// fails), so clean up the process group even if Wait has returned.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	})
	return p
}

func (p *process) output() string {
	data, _ := os.ReadFile(p.log)
	return string(data)
}

func (p *process) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-p.done:
		return p.err
	case <-time.After(operationTimeout):
		t.Fatalf("wrapper did not exit; output: %s", p.output())
		return nil
	}
}

func propertiesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("server-name=original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitHealth(t *testing.T, p *process, addr, path string, status int, body string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.NewTimer(operationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		resp, err := client.Get("http://" + addr + path)
		if err == nil {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			last = fmt.Sprintf("status=%d body=%s read error=%v", resp.StatusCode, data, readErr)
			if resp.StatusCode == status && string(data) == body && readErr == nil {
				return
			}
		} else {
			last = err.Error()
		}
		select {
		case <-p.done:
			t.Fatalf("wrapper exited early: %v; output: %s", p.err, p.output())
		case <-deadline.C:
			t.Fatalf("health %s: %s; output: %s", path, last, p.output())
		case <-ticker.C:
		}
	}
}

func connect(t *testing.T, addr, auth string, expectedStatus int) *websocket.Conn {
	t.Helper()
	headers := http.Header{}
	if auth != "" {
		headers.Set("X-Auth-Key", auth)
	}
	dialer := websocket.Dialer{HandshakeTimeout: operationTimeout}
	conn, resp, err := dialer.Dial("ws://"+addr+"/ws", headers)
	if expectedStatus != http.StatusSwitchingProtocols {
		if conn != nil {
			conn.Close()
		}
		if resp == nil || resp.StatusCode != expectedStatus || err == nil {
			t.Fatalf("websocket rejection: status=%v err=%v, want %d", resp, err, expectedStatus)
		}
		resp.Body.Close()
		return nil
	}
	if err != nil {
		t.Fatalf("websocket connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func send(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	if err := conn.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		t.Fatal(err)
	}
}

func expectMessages(t *testing.T, conn *websocket.Conn, messages ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(messages))
	for _, message := range messages {
		wanted[message] = true
	}
	if err := conn.SetReadDeadline(time.Now().Add(operationTimeout)); err != nil {
		t.Fatal(err)
	}
	for len(wanted) > 0 {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %v: %v", wanted, err)
		}
		delete(wanted, string(data))
	}
}

func TestWrapperSubprocess(t *testing.T) {
	binDir := t.TempDir()
	wrapper := filepath.Join(binDir, "wrapper")
	fake := filepath.Join(binDir, "fake-server")
	build(t, wrapper, "../cmd/minecraft-bedrock-wrapper")
	build(t, fake, "./testdata/fakeserver")

	t.Run("health websocket configuration and clean exit", func(t *testing.T) {
		dir := propertiesDir(t)
		addr := freeAddress(t)
		p := start(t, wrapper, t.TempDir(), []string{"EULA_ACCEPT=true", "CFG_SERVER_NAME=integration", "PORT=19145"},
			[]string{"--command", fake, "--listen", addr, "--app-dir", dir, "--auth-key", key})
		waitHealth(t, p, addr, "/health/live", http.StatusOK, `{"status":"alive"}`)
		waitHealth(t, p, addr, "/health/ready", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
		connect(t, addr, "", http.StatusUnauthorized)
		connect(t, addr, "wrong", http.StatusUnauthorized)
		conn := connect(t, addr, key, http.StatusSwitchingProtocols)
		send(t, conn, "ready")
		expectMessages(t, conn, "Server started")
		waitHealth(t, p, addr, "/health/ready", http.StatusOK, `{"status":"ready"}`)
		send(t, conn, "say hello")
		expectMessages(t, conn, "stdout: say hello", "[ERR] stderr: say hello")
		data, err := os.ReadFile(filepath.Join(dir, "server.properties"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{"server-name=integration\n", "server-port=19145\n", "server-udp-ports=19145:19145\n"} {
			if !strings.Contains(string(data), line) {
				t.Errorf("missing %q in properties: %s", line, data)
			}
		}
		send(t, conn, "stop")
		if err := p.wait(t); err != nil {
			t.Fatalf("clean child exit: %v; output: %s", err, p.output())
		}
	})

	t.Run("child failure propagates", func(t *testing.T) {
		dir := propertiesDir(t)
		addr := freeAddress(t)
		p := start(t, wrapper, dir, []string{"EULA_ACCEPT=true"},
			[]string{"--command", fake, "--listen", addr, "--auth-key", key})
		waitHealth(t, p, addr, "/health/live", http.StatusOK, `{"status":"alive"}`)
		conn := connect(t, addr, key, http.StatusSwitchingProtocols)
		send(t, conn, "fail")
		if err := p.wait(t); err == nil || !strings.Contains(p.output(), "Error running command: exit status 7") {
			t.Fatalf("child failure: err=%v output=%s", err, p.output())
		}
	})

	t.Run("startup errors", func(t *testing.T) {
		cases := []struct {
			name, want string
			env, args  []string
			properties bool
		}{
			{"missing auth", "authentication key is required", []string{"EULA_ACCEPT=true"}, nil, true},
			{"missing EULA", "you must accept the EULA", nil, []string{"--auth-key", key}, true},
			{"missing properties", "error updating server properties", []string{"EULA_ACCEPT=true"}, []string{"--auth-key", key}, false},
			{"missing executable", "Error starting command", []string{"EULA_ACCEPT=true"}, []string{"--auth-key", key, "--command", filepath.Join(binDir, "missing")}, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				if tc.properties {
					dir = propertiesDir(t)
				}
				p := start(t, wrapper, dir, tc.env, tc.args)
				if err := p.wait(t); err == nil || !strings.Contains(p.output(), tc.want) {
					t.Fatalf("startup failure: err=%v output=%s; want %q", err, p.output(), tc.want)
				}
			})
		}
	})
}
