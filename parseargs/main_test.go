package parseargs

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseJustSync(t *testing.T) {
	ia := ParseArgs([]string{"sync"})
	assert.Equal(t, InvokeArgs{
		Action: "sync",
		Files: []string{"secrets.yml"},
	}, ia)
}
