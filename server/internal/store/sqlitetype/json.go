package sqlitetype

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

type JSON []byte

func (j *JSON) Scan(src any) error {
	if j == nil {
		return errors.New("sqlite JSON scan requires a destination")
	}
	if src == nil {
		*j = nil
		return nil
	}
	var value []byte
	switch src := src.(type) {
	case string:
		value = []byte(src)
	case []byte:
		value = append([]byte(nil), src...)
	default:
		return fmt.Errorf("sqlite JSON cannot scan %T", src)
	}
	if !json.Valid(value) {
		return errors.New("sqlite JSON contains invalid JSON")
	}
	*j = value
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, errors.New("sqlite JSON contains invalid JSON")
	}
	return string(j), nil
}

type StringList []string

func (s *StringList) Scan(src any) error {
	if s == nil {
		return errors.New("sqlite string-list scan requires a destination")
	}
	var raw []byte
	switch src := src.(type) {
	case string:
		raw = []byte(src)
	case []byte:
		raw = src
	case nil:
		*s = nil
		return nil
	default:
		return fmt.Errorf("sqlite string list cannot scan %T", src)
	}
	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("sqlite string list: %w", err)
	}
	if value == nil {
		value = []string{}
	}
	*s = value
	return nil
}

func (s StringList) Value() (driver.Value, error) {

	if s == nil {
		s = StringList{}
	}
	value, err := json.Marshal([]string(s))
	if err != nil {
		return nil, fmt.Errorf("sqlite string list: %w", err)
	}
	return string(value), nil
}
