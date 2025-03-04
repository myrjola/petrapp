package envstruct_test

import (
	"errors"
	"github.com/google/go-cmp/cmp"
	"github.com/myrjola/petrapp/internal/envstruct"
	"strings"
	"testing"
)

func TestPopulate(t *testing.T) {
	tests := []struct {
		name      string
		v         any
		lookupEnv func(string) (string, bool)
		want      any
		wantErr   error
	}{
		{
			name:      "nil",
			v:         nil,
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want:      nil,
			wantErr:   envstruct.ErrInvalidValue,
		},
		{
			name:      "not pointer",
			v:         struct{}{},
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want:      nil,
			wantErr:   envstruct.ErrInvalidValue,
		},
		{
			name:      "empty struct",
			v:         &struct{}{},
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want:      &struct{}{},
			wantErr:   nil,
		},
		{
			name: "empty env",
			v: &struct { //nolint:exhaustruct // populated later, populated later
				EnvVar string `env:"ENV_VAR"`
			}{},
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want:      nil,
			wantErr:   envstruct.ErrEnvNotSet,
		},
		{
			name: "env is set",
			v: &struct { //nolint:exhaustruct // populated later, populated later
				EnvVar string `env:"ENV_VAR"`
			}{},
			lookupEnv: func(_ string) (string, bool) { return "env_var", true },
			want: &struct {
				EnvVar string `env:"ENV_VAR"`
			}{EnvVar: "env_var"},
			wantErr: nil,
		},
		{
			name: "picks correct env variable",
			v: &struct { //nolint:exhaustruct // populated later
				EnvVar      string `env:"ENV_VAR"`
				EnvVar2     string `env:"ENV_VAR2"`
				OtherValue  string
				OtherValue2 int
			}{},
			lookupEnv: func(s string) (string, bool) { return strings.ToLower(s), true },
			want: &struct {
				EnvVar      string `env:"ENV_VAR"`
				EnvVar2     string `env:"ENV_VAR2"`
				OtherValue  string
				OtherValue2 int
			}{EnvVar: "env_var", EnvVar2: "env_var2", OtherValue: "", OtherValue2: 0},
			wantErr: nil,
		},
		{
			name: "handles default value",
			v: &struct { //nolint:exhaustruct // populated later
				EnvVarDefault string `env:"ENV_VAR_DEFAULT" envDefault:"default"`
			}{},
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want: &struct {
				EnvVarDefault string `env:"ENV_VAR_DEFAULT" envDefault:"default"`
			}{EnvVarDefault: "default"},
			wantErr: nil,
		},
		{
			name: "only accepts strings",
			v: &struct { //nolint:exhaustruct // populated later
				EnvVar int `env:"ENV_VAR"`
			}{},
			lookupEnv: func(_ string) (string, bool) { return "", false },
			want:      nil,
			wantErr:   envstruct.ErrInvalidValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := envstruct.Populate(tt.v, tt.lookupEnv)

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Populate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Populate() unexpected error = %v", err)
				}
				if diff := cmp.Diff(tt.want, tt.v); diff != "" {
					t.Errorf("Populate() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
