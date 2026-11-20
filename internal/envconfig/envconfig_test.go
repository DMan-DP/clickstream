package envconfig

import (
	"log/slog"
	"slices"
	"testing"
)

func TestList(t *testing.T) {
	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_LIST", tc.in)
			addrs, err := List("TEST_LIST")

			if (err != nil) != tc.wantErr {
				t.Errorf("List(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}

			if err != nil {
				return
			}

			if !slices.Equal(addrs, tc.want) {
				t.Errorf("List(%q) got = %v, want %v", tc.in, addrs, tc.want)
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_LOG_LEVEL", tc.in)
			lvl, err := logLevel("TEST_LOG_LEVEL")

			if (err != nil) != tc.wantErr {
				t.Errorf("logLevel(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}

			if err != nil {
				return
			}

			if lvl != tc.want {
				t.Errorf("logLevel(%q) got = %v, want %v", tc.in, lvl, tc.want)
			}
		})
	}
}
