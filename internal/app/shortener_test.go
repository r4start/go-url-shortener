package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_batchDecodeIDs(t *testing.T) {
	ids := make([]string, 5000)
	for i := 0; i < len(ids); i++ {
		ids[i] = "NWI4NTMwNmZjNWJmMjMzYg"
	}

	tests := []struct {
		name         string
		ids          []string
		workersCount int
	}{
		{
			name:         "Batch decode check #1",
			ids:          ids,
			workersCount: UnlimitedWorkers,
		},
		{
			name:         "Batch decode check #2",
			ids:          make([]string, 0),
			workersCount: UnlimitedWorkers,
		},
		{
			name:         "Batch decode check #3",
			ids:          ids,
			workersCount: MaxWorkersPerRequest,
		},
		{
			name:         "Batch decode check #4",
			ids:          ids,
			workersCount: 17,
		},
		{
			name:         "Batch decode check #5",
			ids:          ids,
			workersCount: len(ids) + 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decodedIds, err := batchDecodeIDs(context.Background(), tt.ids, tt.workersCount)
			assert.Nil(t, err)
			assert.Equal(t, len(tt.ids), len(decodedIds))
		})
	}
}
