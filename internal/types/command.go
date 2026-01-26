package types

import "encoding/json"

type CommandType string

const (
	CmdAllow    CommandType = "ALLOW"
	CmdSetLimit CommandType = "SET_LIMIT"
)

type Command struct {
	Type CommandType `json:"type"`
	Key  string      `json:"key"`
	// The Leader sets this so all nodes
	// apply the rate limit logic at the exact same logical time.
	Timestamp int64   `json:"timestamp"`
	Tokens    uint32  `json:"tokens,omitempty"`
	Rate      float64 `json:"rate,omitempty"`
	Burst     uint32  `json:"burst,omitempty"`
}

func (c Command) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func UnmarshalCommand(data []byte) (Command, error) {
	var c Command
	err := json.Unmarshal(data, &c)
	return c, err
}
