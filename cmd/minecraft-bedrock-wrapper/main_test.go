package main

import (
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRun_HelpReturnsZero(t *testing.T) {
	t.Parallel()

	if got := run([]string{"--help"}); got != 0 {
		t.Fatalf("run(--help) = %d; want 0", got)
	}
}

func TestLoadFlagsFromEnvUsesMinecraftVersionEnv(t *testing.T) {
	t.Setenv("MINECRAFT_VERSION", "1.2.3.4")

	listenAddress := ""
	appDir := ""
	mcVersion := ""
	authKey := ""
	port := ""

	loadFlagsFromEnv(&listenAddress, &appDir, &mcVersion, &authKey, &port)
	if mcVersion != "1.2.3.4" {
		t.Fatalf("expected MINECRAFT_VERSION to be loaded, got %q", mcVersion)
	}
}

func TestLoadFlagsFromEnvUsesPortEnv(t *testing.T) {
	t.Setenv("PORT", "19140")

	listenAddress := ""
	appDir := ""
	mcVersion := ""
	authKey := ""
	port := ""

	loadFlagsFromEnv(&listenAddress, &appDir, &mcVersion, &authKey, &port)
	if port != "19140" {
		t.Fatalf("expected PORT to be loaded, got %q", port)
	}
}

func TestValidateRuntimeConfigRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		mcVersion string
		authKey   string
		eula      string
		wantErr   string
	}{
		{
			name:    "missing Minecraft version",
			authKey: "secret",
			eula:    "true",
			wantErr: "Minecraft version is required",
		},
		{
			name:      "invalid Minecraft version",
			mcVersion: "1.2.3",
			authKey:   "secret",
			eula:      "true",
			wantErr:   "invalid Minecraft version format",
		},
		{
			name:      "missing authentication key",
			mcVersion: "1.2.3.4",
			eula:      "true",
			wantErr:   "authentication key is required",
		},
		{
			name:      "EULA not accepted",
			mcVersion: "1.2.3.4",
			authKey:   "secret",
			eula:      "false",
			wantErr:   "you must accept the EULA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EULA_ACCEPT", tt.eula)

			mcVersion := tt.mcVersion
			authKey := tt.authKey
			port := ""
			err := validateRuntimeConfig(&mcVersion, &authKey, &port)
			if err == nil {
				t.Fatal("validateRuntimeConfig() error = nil; want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRuntimeConfig() error = %q; want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRuntimeConfigSetsRuntimeEnvironment(t *testing.T) {
	t.Setenv("EULA_ACCEPT", "true")
	t.Setenv("PORT", "19132")
	t.Setenv("LD_LIBRARY_PATH", "previous")

	mcVersion := "1.2.3.4"
	authKey := "secret"
	port := "19140"
	if err := validateRuntimeConfig(&mcVersion, &authKey, &port); err != nil {
		t.Fatalf("validateRuntimeConfig() error = %v; want nil", err)
	}
	if got := os.Getenv("PORT"); got != "19140" {
		t.Fatalf("PORT = %q; want %q", got, "19140")
	}
	if got := os.Getenv("LD_LIBRARY_PATH"); got != "." {
		t.Fatalf("LD_LIBRARY_PATH = %q; want %q", got, ".")
	}
}

func TestValidateRuntimeConfigPreservesPortWhenUnset(t *testing.T) {
	t.Setenv("EULA_ACCEPT", "true")
	t.Setenv("PORT", "19132")

	mcVersion := "1.2.3.4"
	authKey := "secret"
	port := ""
	if err := validateRuntimeConfig(&mcVersion, &authKey, &port); err != nil {
		t.Fatalf("validateRuntimeConfig() error = %v; want nil", err)
	}
	if got := os.Getenv("PORT"); got != "19132" {
		t.Fatalf("PORT = %q; want %q", got, "19132")
	}
}

func TestResolveBuildMetadata(t *testing.T) {
	t.Parallel()

	info := func(version, commit, date string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Version: version},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: commit},
				{Key: "vcs.time", Value: date},
			},
		}
	}

	tests := []struct {
		name     string
		info     *debug.BuildInfo
		metadata buildMetadata
		want     buildMetadata
	}{
		{
			name:     "build info fills defaults",
			info:     info("v1.2.3", "abc123", "2026-01-02T03:04:05Z"),
			metadata: buildMetadata{version: "dev", commit: "none", date: "unknown"},
			want:     buildMetadata{version: "v1.2.3", commit: "abc123", date: "2026-01-02T03:04:05Z"},
		},
		{
			name:     "ldflags take precedence",
			info:     info("v1.2.3", "abc123", "2026-01-02T03:04:05Z"),
			metadata: buildMetadata{version: "v9.8.7", commit: "def456", date: "2025-06-07T08:09:10Z"},
			want:     buildMetadata{version: "v9.8.7", commit: "def456", date: "2025-06-07T08:09:10Z"},
		},
		{
			name:     "build info fills only default fields",
			info:     info("v1.2.3", "abc123", "2026-01-02T03:04:05Z"),
			metadata: buildMetadata{version: "v9.8.7", commit: "none", date: "2025-06-07T08:09:10Z"},
			want:     buildMetadata{version: "v9.8.7", commit: "abc123", date: "2025-06-07T08:09:10Z"},
		},
		{
			name:     "devel version is ignored",
			info:     info("(devel)", "abc123", "2026-01-02T03:04:05Z"),
			metadata: buildMetadata{version: "dev", commit: "none", date: "unknown"},
			want:     buildMetadata{version: "dev", commit: "abc123", date: "2026-01-02T03:04:05Z"},
		},
		{
			name:     "empty version is ignored",
			info:     info("", "abc123", "2026-01-02T03:04:05Z"),
			metadata: buildMetadata{version: "dev", commit: "none", date: "unknown"},
			want:     buildMetadata{version: "dev", commit: "abc123", date: "2026-01-02T03:04:05Z"},
		},
		{
			name:     "nil build info preserves linked metadata",
			info:     nil,
			metadata: buildMetadata{version: "v9.8.7", commit: "def456", date: "2025-06-07T08:09:10Z"},
			want:     buildMetadata{version: "v9.8.7", commit: "def456", date: "2025-06-07T08:09:10Z"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveBuildMetadata(tt.info, tt.metadata); got != tt.want {
				t.Fatalf("resolveBuildMetadata() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
