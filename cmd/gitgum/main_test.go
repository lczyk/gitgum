package main

import (
	"reflect"
	"testing"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/assert"
)

func TestHoistFollow(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"gg", "-f", "status"}, []string{"gg", "status", "-f"}},
		{[]string{"gg", "--follow", "tree"}, []string{"gg", "tree", "--follow"}},
		{[]string{"gg", "-f=5", "status"}, []string{"gg", "status", "-f=5"}},
		{[]string{"gg", "-f", "tree", "-f"}, []string{"gg", "tree", "-f", "-f"}}, // double -f == single
		{[]string{"gg", "-f", "foo"}, []string{"gg", "foo", "-f"}},               // unknown-cmd errors like `gg foo -f`
		{[]string{"gg", "status", "-f"}, []string{"gg", "status", "-f"}},         // already correct, untouched
		{[]string{"gg", "status"}, []string{"gg", "status"}},                     // no follow, untouched
		{[]string{"gg", "-f"}, []string{"gg", "-f"}},                             // no command, untouched
		{[]string{"gg", "--version", "status"}, []string{"gg", "--version", "status"}}, // non-follow flag bails
	}
	for _, c := range cases {
		got := hoistFollow(c.in)
		assert.That(t, reflect.DeepEqual(got, c.want), "hoistFollow(%v) = %v, want %v", c.in, got, c.want)
	}
}

// Ensures every field of Options implements flags.Commander. A wrong Execute
// signature would otherwise silently become a no-op at runtime (command parses,
// exits 0, prints nothing).
func TestAllCommandsImplementCommander(t *testing.T) {
	commanderType := reflect.TypeOf((*flags.Commander)(nil)).Elem()
	optsType := reflect.TypeOf(Options{})

	assert.That(t, optsType.NumField() > 0, "Options has no fields")

	for i := 0; i < optsType.NumField(); i++ {
		field := optsType.Field(i)

		// go-flags silently ignores fields without the command: tag, so they'd
		// never actually be registered — catch that here.
		assert.That(t, field.Tag.Get("command") != "",
			"%s: missing command: struct tag — go-flags will silently skip this field", field.Name)

		// go-flags accepts a missing description: tag but shows empty help text — catch that here.
		assert.That(t, field.Tag.Get("description") != "",
			"%s: missing description: struct tag — command will have empty help text in gg --help", field.Name)

		ptrType := reflect.PointerTo(field.Type)
		assert.That(t, ptrType.Implements(commanderType),
			"%s (%s) does not implement flags.Commander — check Execute signature is Execute(args []string) error", field.Name, field.Type)
	}
}
