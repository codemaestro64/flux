package types

import (
	"bytes"
	"encoding/gob"
)

type CommandType int

const (
	CmdAllow CommandType = iota
	CmdSetLimit
)

type Command struct {
	Type      CommandType
	Key       string
	Tokens    uint32
	Rate      float64
	Burst     uint32
	Timestamp int64
}

func (c *Command) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func UnmarshalCommand(data []byte) (*Command, error) {
	var cmd Command
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&cmd); err != nil {
		return nil, err
	}
	return &cmd, nil
}
