package workflow

import "encoding/json"

var _ JSONSerialisable = (*ParticipantID)(nil)

func (pid ParticipantID) MarshalJSON() (_ []byte, _ error) {
	return json.Marshal(string(pid))
}

func (pid *ParticipantID) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*pid = ParticipantID(v)
	return nil
}

var _ JSONSerialisable = (*Sequence)(nil)

func (s Sequence) MarshalJSON() ([]byte, error) {
	type T Sequence
	return json.Marshal(T(s))
}

func (s *Sequence) UnmarshalJSON(data []byte) error {
	type T Sequence
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = Sequence(v)
	return nil
}

var _ JSONSerialisable = (*If)(nil)

func (d If) MarshalJSON() ([]byte, error) {
	type T If
	return json.Marshal(T(d))
}

func (d *If) UnmarshalJSON(data []byte) error {
	type T If
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*d = If(v)
	return nil
}

var _ JSONSerialisable = (*State)(nil)

func (s State) MarshalJSON() ([]byte, error) {
	type DTO State
	return json.Marshal(DTO(s))
}

func (s *State) UnmarshalJSON(data []byte) error {
	type DTO State
	var dto DTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*s = State(dto)
	return nil
}

var _ JSONSerialisable = (*Process)(nil)

func (p Process) MarshalJSON() ([]byte, error) {
	type DTO Process
	return json.Marshal(DTO(p))
}

func (p *Process) UnmarshalJSON(data []byte) error {
	type DTO Process
	var dto DTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*p = Process(dto)
	return nil
}
