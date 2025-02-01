package envstruct_test

import (
	"github.com/myrjola/petrapp/internal/envstruct"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestPopulate(t *testing.T) {
	type args struct {
		v         any
		lookupEnv func(string) (string, bool)
	}
	tests := []struct {
		name    string
		args    args
		want    any
		wantErr error
	}{
		{
			name: "nil",
			args: args{
				v:         nil,
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want:    nil,
			wantErr: envstruct.ErrInvalidValue,
		},
		{
			name: "not pointer",
			args: args{
				v:         struct{}{},
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want:    nil,
			wantErr: envstruct.ErrInvalidValue,
		},
		{
			name: "empty struct",
			args: args{
				v:         &struct{}{},
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want:    &struct{}{},
			wantErr: nil,
		},
		{
			name: "empty env",
			args: args{
				v: &struct { //nolint:exhaustruct // populated later
					EnvVar string `env:"ENV_VAR"`
				}{},
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want:    nil,
			wantErr: envstruct.ErrEnvNotSet,
		},
		{
			name: "env is set",
			args: args{
				v: &struct { //nolint:exhaustruct // populated later
					EnvVar string `env:"ENV_VAR"`
				}{},
				lookupEnv: func(_ string) (string, bool) { return "env_var", true },
			},
			want:    &struct{ EnvVar string }{EnvVar: "env_var"},
			wantErr: nil,
		},
		{
			name: "picks correct env variable",
			args: args{
				v: &struct { //nolint:exhaustruct // populated later
					EnvVar      string `env:"ENV_VAR"`
					EnvVar2     string `env:"ENV_VAR2"`
					OtherValue  string
					OtherValue2 int
				}{},
				lookupEnv: func(s string) (string, bool) { return strings.ToLower(s), true },
			},
			want: &struct {
				EnvVar      string
				EnvVar2     string
				OtherValue  string
				OtherValue2 int
			}{EnvVar: "env_var", EnvVar2: "env_var2", OtherValue: "", OtherValue2: 0},
			wantErr: nil,
		},
		{
			name: "handles default value",
			args: args{
				v: &struct { //nolint:exhaustruct // populated later
					EnvVarDefault string `env:"ENV_VAR_DEFAULT" envDefault:"default"`
				}{},
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want: &struct {
				EnvVarDefault string
			}{EnvVarDefault: "default"},
			wantErr: nil,
		},
		{
			name: "only accepts strings",
			args: args{
				v: &struct { //nolint:exhaustruct // populated later
					EnvVar int `env:"ENV_VAR"`
				}{},
				lookupEnv: func(_ string) (string, bool) { return "", false },
			},
			want:    nil,
			wantErr: envstruct.ErrInvalidValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.args.v
			err := envstruct.Populate(v, tt.args.lookupEnv)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, tt.want, v)
			}
		})
	}
}
