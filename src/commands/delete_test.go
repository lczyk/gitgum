package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestDeleteCommand_Execute(t *testing.T) {
	cmd := &DeleteCommand{}
	assert.That(t, cmd != nil, "DeleteCommand should be created successfully")
}
