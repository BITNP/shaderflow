package archive

import (
	"encoding/json"
	"fmt"
	"shaderflow/internal/game"
)

type Checkpoint struct {
	Seq   uint64     `json:"seq"`
	State game.State `json:"state"`
}

func ReplayFrom(start game.State, events []game.Envelope, checkpoints []Checkpoint, through uint64) (game.State, error) {
	base := start
	from := uint64(0)
	for _, checkpoint := range checkpoints {
		if checkpoint.Seq <= through && checkpoint.Seq >= from {
			base = checkpoint.State
			from = checkpoint.Seq
		}
	}
	remaining := make([]game.Envelope, 0)
	for _, event := range events {
		if event.Seq > from {
			remaining = append(remaining, event)
		}
	}
	return Replay(base, remaining, through)
}

func Replay(start game.State, events []game.Envelope, through uint64) (game.State, error) {
	state := cloneState(start)
	if state.Codes == nil {
		state.Codes = map[string]game.Code{}
	}
	for _, envelope := range events {
		if envelope.Seq > through {
			break
		}
		if envelope.Event == nil {
			continue
		}
		switch envelope.Event.Type {
		case "code.updated":
			var payload struct {
				NodeID string    `json:"nodeId"`
				Code   game.Code `json:"code"`
			}
			if err := convert(envelope.Event.Data, &payload); err != nil {
				return state, fmt.Errorf("code.updated: %w", err)
			}
			state.Codes[payload.NodeID] = payload.Code
		case "input.updated":
			var input game.ExternalInput
			if err := convert(envelope.Event.Data, &input); err != nil {
				return state, fmt.Errorf("input.updated: %w", err)
			}
			if state.ExternalInputs == nil {
				state.ExternalInputs = map[string]game.ExternalInput{}
			}
			state.ExternalInputs[input.ID] = input
		case "snapshot.created":
			var snapshot game.RenderSnapshot
			if err := convert(envelope.Event.Data, &snapshot); err != nil {
				return state, fmt.Errorf("snapshot.created: %w", err)
			}
			state.RenderSnapshots = append(state.RenderSnapshots, snapshot)
		case "time.controlled":
			var control game.TimeControl
			if err := convert(envelope.Event.Data, &control); err != nil {
				return state, fmt.Errorf("time.controlled: %w", err)
			}
			state.TimeControl = &control
		case "trajectory.updated":
			var trajectory game.Trajectory
			if err := convert(envelope.Event.Data, &trajectory); err != nil {
				return state, fmt.Errorf("trajectory.updated: %w", err)
			}
			if trajectory.Active {
				state.Trajectory = &trajectory
			} else {
				state.Trajectory = nil
			}
		case "round.terminated":
			state.Phase = "registration"
		}
		state.Seq = envelope.Seq
	}
	return state, nil
}
func cloneState(value game.State) game.State {
	data, _ := json.Marshal(value)
	var result game.State
	_ = json.Unmarshal(data, &result)
	return result
}
func convert(from, to any) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}
