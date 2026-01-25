package types

import "encoding/json"

type CommandType string

const (
	CmdAllow    CommandType = "ALLOW"
	CmdSetLimit CommandType = "SET_LIMIT"
)

type Command struct {
	Type   CommandType `json:"type"`
	Key    string      `json:"key"`
	Tokens uint32      `json:"tokens,omitempty"` // for Allow
	Rate   float64     `json:"rate,omitempty"`   // tokens per second
	Burst  uint32      `json:"burst,omitempty"`
}

func (c Command) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func UnmarshalCommand(data []byte) (Command, error) {
	var c Command
	err := json.Unmarshal(data, &c)
	return c, err
}
