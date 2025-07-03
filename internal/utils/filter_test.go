package utils

import (
	"testing"
)

func TestBuildGcpLabelFilter(t *testing.T) {
	tests := []struct {
		name        string
		labelFilter string
		want        string
		wantErr     bool
	}{
		{
			name:        "empty filter",
			labelFilter: "",
			want:        "",
			wantErr:     false,
		},
		{
			name:        "simple key=value",
			labelFilter: "env=production",
			want:        "labels.env=\"production\"",
			wantErr:     false,
		},
		{
			name:        "value with spaces",
			labelFilter: "app-version=1.0 beta",
			want:        "labels.app-version=\"1.0 beta\"",
			wantErr:     false,
		},
		{
			name:        "value with special characters",
			labelFilter: "description=This is a test/demo",
			want:        "labels.description=\"This is a test/demo\"",
			wantErr:     false,
		},
		{
			name:        "key only (presence check)",
			labelFilter: "backup",
			want:        "labels.backup:*",
			wantErr:     false,
		},
		{
			name:        "key with hyphen only",
			labelFilter: "has-backup",
			want:        "labels.has-backup:*",
			wantErr:     false,
		},
		{
			name:        "empty key",
			labelFilter: "=value",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "key with spaces trimmed",
			labelFilter: "  env  =  production  ",
			want:        "labels.env=\"production\"",
			wantErr:     false,
		},
		{
			name:        "empty value",
			labelFilter: "env=",
			want:        "labels.env=\"\"",
			wantErr:     false,
		},
		{
			name:        "value with double quotes",
			labelFilter: `message=hello "world"`,
			want:        `labels.message="hello \"world\""`,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildGcpLabelFilter(tt.labelFilter)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildGcpLabelFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BuildGcpLabelFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesLabel(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		labelFilter string
		want        bool
		wantErr     bool
	}{
		{
			name:        "empty filter matches everything",
			labels:      map[string]string{"env": "production"},
			labelFilter: "",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "exact match",
			labels:      map[string]string{"env": "production"},
			labelFilter: "env=production",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "no match - different value",
			labels:      map[string]string{"env": "production"},
			labelFilter: "env=staging",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "no match - missing key",
			labels:      map[string]string{"env": "production"},
			labelFilter: "region=us-east1",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "presence check - exists",
			labels:      map[string]string{"backup": "enabled"},
			labelFilter: "backup",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "presence check - not exists",
			labels:      map[string]string{"env": "production"},
			labelFilter: "backup",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "value with spaces",
			labels:      map[string]string{"app-version": "1.0 beta"},
			labelFilter: "app-version=1.0 beta",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "empty labels map",
			labels:      map[string]string{},
			labelFilter: "env=production",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "nil labels map",
			labels:      nil,
			labelFilter: "env=production",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "empty key error",
			labels:      map[string]string{"env": "production"},
			labelFilter: "=value",
			want:        false,
			wantErr:     true,
		},
		{
			name:        "trimmed spaces",
			labels:      map[string]string{"env": "production"},
			labelFilter: "  env  =  production  ",
			want:        true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchesLabel(tt.labels, tt.labelFilter)
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchesLabel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MatchesLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}