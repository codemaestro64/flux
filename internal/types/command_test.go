package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		command Command
	}{
		{
			name: "Allow Command",
			command: Command{
				Type:      CmdAllow,
				Key:       "user_123",
				Tokens:    5,
				Timestamp: time.Now().UnixNano(),
			},
		},
		{
			name: "SetLimit Command",
			command: Command{
				Type:      CmdSetLimit,
				Key:       "api_gateway",
				Rate:      100.5,
				Burst:     500,
				Timestamp: time.Now().UnixNano(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the command
			data, err := tt.command.Marshal()
			require.NoError(t, err, "failed to marshal command")
			require.NotNil(t, data)

			// Unmarshal the data back into a new Command
			got, err := UnmarshalCommand(data)
			require.NoError(t, err, "failed to unmarshal command")

			// Compare the original with the result
			assert.Equal(t, tt.command.Type, got.Type)
			assert.Equal(t, tt.command.Key, got.Key)
			assert.Equal(t, tt.command.Tokens, got.Tokens)
			assert.Equal(t, tt.command.Rate, got.Rate)
			assert.Equal(t, tt.command.Burst, got.Burst)
			assert.Equal(t, tt.command.Timestamp, got.Timestamp)
		})
	}
}

func TestUnmarshalCommand_Error(t *testing.T) {
	// Ensure that passing garbage data returns an error instead of panicking
	garbage := []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF}
	cmd, err := UnmarshalCommand(garbage)

	assert.Error(t, err)
	assert.Nil(t, cmd)
}
