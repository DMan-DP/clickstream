package config

import (
	"slices"
	"testing"
)

func TestHttpAddr(t *testing.T) {
}

func TestBroker(t *testing.T) {
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
			t.Setenv("KAFKA_BROKERS", config.in)
			addrs, err := getKafkaBrokers()

			if (err != nil) != config.wantErr {
				t.Errorf("getKafkaBrokers(%q) err = %v, wantErr %v", config.in, err, config.wantErr)
			}

			if err != nil {
				return
			}

			if !slices.Equal(addrs, config.want) {
				t.Errorf("getKafkaBrokers(%q) got = %v, want %v", config.in, addrs, config.want)
			}
		})
	}
}
