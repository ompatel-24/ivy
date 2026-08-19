package command

import (
	"errors"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Invocation
		wantErr error
	}{
		{
			name:    "missing command",
			wantErr: ErrMissingCommand,
		},
		{
			name: "command",
			args: []string{"foo"},
			want: Invocation{Action: ActionRun, Args: []string{"foo"}},
		},
		{
			name: "command arguments",
			args: []string{"foo", "bar", "--baz"},
			want: Invocation{Action: ActionRun, Args: []string{"foo", "bar", "--baz"}},
		},
		{
			name: "separator",
			args: []string{"--", "foo", "--bar", "baz"},
			want: Invocation{Action: ActionRun, Args: []string{"foo", "--bar", "baz"}},
		},
		{
			name: "separator permits reserved command",
			args: []string{"--", "help"},
			want: Invocation{Action: ActionRun, Args: []string{"help"}},
		},
		{
			name:    "empty separator",
			args:    []string{"--"},
			wantErr: ErrMissingCommand,
		},
		{
			name: "help",
			args: []string{"help"},
			want: Invocation{Action: ActionHelp},
		},
		{
			name: "short help",
			args: []string{"-h"},
			want: Invocation{Action: ActionHelp},
		},
		{
			name: "long help",
			args: []string{"--help"},
			want: Invocation{Action: ActionHelp},
		},
		{
			name: "version",
			args: []string{"version"},
			want: Invocation{Action: ActionVersion},
		},
		{
			name: "long version",
			args: []string{"--version"},
			want: Invocation{Action: ActionVersion},
		},
		{
			name:    "help arguments",
			args:    []string{"help", "extra"},
			wantErr: errors.New("help does not accept arguments"),
		},
		{
			name:    "version arguments",
			args:    []string{"--version", "extra"},
			wantErr: errors.New("--version does not accept arguments"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse(test.args)
			if test.wantErr != nil {
				if err == nil {
					t.Fatalf("Parse() error = nil, want %q", test.wantErr)
				}
				if errors.Is(test.wantErr, ErrMissingCommand) {
					if !errors.Is(err, ErrMissingCommand) {
						t.Fatalf("Parse() error = %v, want ErrMissingCommand", err)
					}
				} else if err.Error() != test.wantErr.Error() {
					t.Fatalf("Parse() error = %q, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Parse() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseClonesChildArguments(t *testing.T) {
	args := []string{"foo", "bar"}
	invocation, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	args[0] = "changed"
	if invocation.Args[0] != "foo" {
		t.Fatalf("Parse() retained caller's backing array: %q", invocation.Args[0])
	}
}
