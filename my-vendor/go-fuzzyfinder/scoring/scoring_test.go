package scoring

import "testing"

func TestCalculate(t *testing.T) {
	t.Parallel()

	t.Run("no panic with equal-length strings", func(t *testing.T) {
		t.Parallel()
		Calculate("foo", "foo")
	})

	t.Run("no panic with empty strings", func(t *testing.T) {
		t.Parallel()
		Calculate("", "")
	})

	t.Run("panics when s2 longer than s1", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Error("Calculate must panic when len(s2) > len(s1)")
			}
		}()
		Calculate("foo", "foobar")
	})
}
