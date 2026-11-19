package envconfig

import (
	"log/slog"
	"slices"
	"testing"
)

func TestList(t *testing.T) {
	configs := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{
			name:    "ok",
			in:      "a:9091, b:9092",
			want:    []string{"a:9091", "b:9092"},
			wantErr: false,
		},
		{
			name:    "single addres",
			in:      "a:9091",
			want:    []string{"a:9091"},
			wantErr: false,
		},
		{
			name:    "empty element",
			in:      "a:9091,,b:9092",
			want:    []string{"a:9091", "b:9092"},
			wantErr: false,
		},
		{
			name:    "empty second element",
			in:      "a:9091,",
			want:    []string{"a:9091"},
			wantErr: false,
		},
		{
			name:    "empty config",
			in:      "",
			want:    []string{},
			wantErr: true,
		},
		{
			name:    "only spaces",
			in:      " ",
			want:    []string{},
			wantErr: true,
		},
		{
			name:    "has no addresses",
			in:      ",",
			want:    []string{},
			wantErr: true,
		},
	}

	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			t.Setenv("TEST_LIST", config.in)
			addrs, err := List("TEST_LIST")

			if (err != nil) != config.wantErr {
				t.Errorf("List(%q) err = %v, wantErr %v", config.in, err, config.wantErr)
			}

			if err != nil {
				return
			}

			if !slices.Equal(addrs, config.want) {
				t.Errorf("List(%q) got = %v, want %v", config.in, addrs, config.want)
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	configs := []struct {
		name    string
		in      string
		want    slog.Level
		wantErr bool
	}{
		{
			name:    "debug",
			in:      "debug",
			want:    slog.LevelDebug,
			wantErr: false,
		},
		{
			name:    "info",
			in:      "info",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "warn",
			in:      "warn",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "error",
			in:      "error",
			want:    slog.LevelError,
			wantErr: false,
		},
		{
			name:    "empty uses default",
			in:      "",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "upper case",
			in:      "DEBUG",
			wantErr: true,
		},
		{
			name:    "unknown level",
			in:      "verbose",
			wantErr: true,
		},
		{
			name:    "surrounding spaces",
			in:      " debug ",
			wantErr: true,
		},
	}

	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			t.Setenv("TEST_LOG_LEVEL", config.in)
			lvl, err := logLevel("TEST_LOG_LEVEL")

			if (err != nil) != config.wantErr {
				t.Errorf("logLevel(%q) err = %v, wantErr %v", config.in, err, config.wantErr)
			}

			if err != nil {
				return
			}

			if lvl != config.want {
				t.Errorf("logLevel(%q) got = %v, want %v", config.in, lvl, config.want)
			}
		})
	}
}
