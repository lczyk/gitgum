package scoring

import "testing"

func TestCalculate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		s1, s2    string
		willPanic bool
	}{
		"must not panic":  {s1: "foo", s2: "foo"},
		"must not panic2": {s1: "", s2: ""},
		"must panic":      {s1: "foo", s2: "foobar", willPanic: true},
	}

	for _, c := range cases {
		if c.willPanic {
			defer func() {
				if err := recover(); err == nil {
					t.Error("Calculate must panic")
				}
			}()
		}
		Calculate(c.s1, c.s2)
	}
}

