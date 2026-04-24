package scoring

import "testing"

func TestCalculate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		s1, s2  string
		wantErr bool
	}{
		{"equal-length strings", "foo", "foo", false},
		{"empty strings", "", "", false},
		{"s2 longer than s1", "foo", "foobar", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Calculate(c.s1, c.s2)
			if c.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
