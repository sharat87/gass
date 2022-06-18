package github

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCollectOneSecret(t *testing.T) {
	filesBySecret := CollectFilesBySecret(map[string][]byte{
		"one.yaml": []byte("some random yaml content: ${{ secrets.ONE_SECRET }} here"),
	})
	assert.Equal(t, map[string]map[string]interface{}{
		"ONE_SECRET": map[string]interface{}{
			"one.yaml": nil,
		},
	}, filesBySecret)
}
