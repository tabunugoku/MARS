package httpapi

import "encoding/json"

// Opt is an optional, nullable field of a PATCH body: Set is false when the
// field was absent, and Value is nil when it was null (which clears it).
type Opt[T any] struct {
	Set   bool
	Value *T
}

func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set, o.Value = true, nil
	if string(b) == "null" {
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

func (o Opt[T]) MarshalJSON() ([]byte, error) { return json.Marshal(o.Value) }
