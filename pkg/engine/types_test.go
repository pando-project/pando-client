package engine

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseAndUnparseInclusion(t *testing.T) {
	a := &MetaInclusion{
		Provider:       "",
		InPando:        false,
		InSnapShot:     false,
		SnapShotHeight: 0,
		Context:        nil,
		TranscationID:  0,
	}

	b, err := json.Marshal(a)
	assert.NoError(t, err)

	var c MetaInclusion
	err = json.Unmarshal(b, &c)
	assert.NoError(t, err)

}
