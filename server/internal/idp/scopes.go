package idp

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type Scopes []string

func (scopes *Scopes) Scan(value any) error {
	var encoded []byte
	switch value := value.(type) {
	case string:
		encoded = []byte(value)
	case []byte:
		encoded = value
	default:
		return fmt.Errorf("scan OIDC scopes from %T", value)
	}
	var decoded []string
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Errorf("decode OIDC scopes: %w", err)
	}
	*scopes = decoded
	return nil
}

func (scopes Scopes) Value() (driver.Value, error) {
	encoded, err := json.Marshal([]string(scopes))
	if err != nil {
		return nil, fmt.Errorf("encode OIDC scopes: %w", err)
	}
	return string(encoded), nil
}
