package main

import (
	"reflect"
	"testing"

	flags "github.com/jessevdk/go-flags"
)

// Ensures every field of Options implements flags.Commander. A wrong Execute
// signature would otherwise silently become a no-op at runtime (command parses,
// exits 0, prints nothing).
func TestAllCommandsImplementCommander(t *testing.T) {
	commanderType := reflect.TypeOf((*flags.Commander)(nil)).Elem()
	optsType := reflect.TypeOf(Options{})

	if optsType.NumField() == 0 {
		t.Fatal("Options has no fields")
	}

	for i := 0; i < optsType.NumField(); i++ {
		field := optsType.Field(i)
		ptrType := reflect.PointerTo(field.Type)
		if !ptrType.Implements(commanderType) {
			t.Errorf("%s (%s) does not implement flags.Commander — check Execute signature is Execute(args []string) error", field.Name, field.Type)
		}
	}
}
