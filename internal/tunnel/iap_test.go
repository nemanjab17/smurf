package tunnel

import (
	"testing"
)

func TestParseIAPConfig(t *testing.T) {
	tests := []struct {
		input   string
		want    IAPConfig
		wantErr bool
	}{
		{
			input: "my-instance:us-central1-a:my-project",
			want:  IAPConfig{Instance: "my-instance", Zone: "us-central1-a", Project: "my-project"},
		},
		{
			input:   "only-two:parts",
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
		{
			input:   "::project",
			wantErr: true,
		},
		{
			input:   "instance:zone:",
			wantErr: true,
		},
		{
			input: "inst:zone:proj:extra:colons",
			want:  IAPConfig{Instance: "inst", Zone: "zone", Project: "proj:extra:colons"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseIAPConfig(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
