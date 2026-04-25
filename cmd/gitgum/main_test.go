package main

import (
	"reflect"
	"testing"

	flags "github.com/jessevdk/go-flags"
	"github.com/lczyk/assert"
)

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

		ptrType := reflect.PointerTo(field.Type)
		assert.That(t, ptrType.Implements(commanderType),
			"%s (%s) does not implement flags.Commander — check Execute signature is Execute(args []string) error", field.Name, field.Type)
	}
}
