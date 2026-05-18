package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoCopiesPrompt(t *testing.T) {
	task := &model.Task{
		Prompt: "draw this image",
	}

	got := TaskModel2Dto(task)

	require.Equal(t, "draw this image", got.Prompt)
}
