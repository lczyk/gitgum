package scoring

import "testing"

func TestCalculate(t *testing.T) {
	t.Parallel()

	t.Run("equal-length strings", func(t *testing.T) {
		t.Parallel()
		_, _, err := Calculate("foo", "foo")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty strings", func(t *testing.T) {
		t.Parallel()
		_, _, err := Calculate("", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("error when s2 longer than s1", func(t *testing.T) {
		t.Parallel()
		_, _, err := Calculate("foo", "foobar")
		if err == nil {
			t.Error("Calculate must return error when s2 is longer than s1")
		}
	})
}
