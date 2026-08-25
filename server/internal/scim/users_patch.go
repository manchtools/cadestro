package scim

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	errPatchActiveValue   = errors.New("the active value must be a boolean")
	errPatchUserNameValue = errors.New("the userName value must be a non-empty string")
	errPatchEmailsValue   = errors.New("the emails value must be a non-empty array of email objects")
	errPatchNameValue     = errors.New("the name value must be an object")
	errPatchNoPathValue   = errors.New("a replace without a path requires an object value")
)

func applyUserPatchOp(a *subjectAssertion, op SCIMPatchOp) error {
	switch strings.ToLower(strings.TrimSpace(op.Path)) {
	case "active":
		active, err := patchBool(op.Value)
		if err != nil {
			return err
		}
		a.Active = &active

	case "username":
		value, ok := patchString(op.Value)
		if !ok || value == "" {
			return errPatchUserNameValue
		}
		email := normalizeEmail(value)
		a.Email = &email

	case "emails":
		email, err := patchPrimaryEmail(op.Value)
		if err != nil {
			return err
		}
		a.Email = &email

	case "name":
		var fields map[string]json.RawMessage
		if json.Unmarshal(op.Value, &fields) != nil || fields == nil {
			return errPatchNameValue
		}
		if v, ok := patchString(fields["givenName"]); ok {
			a.GivenName = &v
		}
		if v, ok := patchString(fields["familyName"]); ok {
			a.FamilyName = &v
		}
		if v, ok := patchString(fields["formatted"]); ok {
			a.DisplayName = &v
		}

	case "name.givenname":
		v, ok := patchString(op.Value)
		if !ok {
			return errPatchNameValue
		}
		a.GivenName = &v

	case "name.familyname":
		v, ok := patchString(op.Value)
		if !ok {
			return errPatchNameValue
		}
		a.FamilyName = &v

	case "name.formatted":
		v, ok := patchString(op.Value)
		if !ok {
			return errPatchNameValue
		}
		a.DisplayName = &v

	case "":
		var fields map[string]json.RawMessage
		if json.Unmarshal(op.Value, &fields) != nil || fields == nil {
			return errPatchNoPathValue
		}
		for key, value := range fields {
			if err := applyUserPatchOp(a, SCIMPatchOp{
				Op:    SCIMPatchOpReplace,
				Path:  key,
				Value: value,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func patchBool(value json.RawMessage) (bool, error) {
	var boolean *bool
	if json.Unmarshal(value, &boolean) == nil && boolean != nil {
		return *boolean, nil
	}
	var text *string
	if json.Unmarshal(value, &text) == nil && text != nil {
		switch strings.ToLower(strings.TrimSpace(*text)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, errPatchActiveValue
}

func patchString(value json.RawMessage) (string, bool) {
	var text *string
	if json.Unmarshal(value, &text) != nil || text == nil {
		return "", false
	}
	return *text, true
}

func patchPrimaryEmail(value json.RawMessage) (string, error) {
	var entries []json.RawMessage
	if json.Unmarshal(value, &entries) != nil || len(entries) == 0 {
		return "", errPatchEmailsValue
	}
	chosen := ""
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		if json.Unmarshal(raw, &entry) != nil || entry == nil {
			continue
		}
		address, ok := patchString(entry["value"])
		if !ok || address == "" {
			continue
		}
		var primary bool
		if json.Unmarshal(entry["primary"], &primary) == nil && primary {
			return normalizeEmail(address), nil
		}
		if chosen == "" {
			chosen = address
		}
	}
	if chosen == "" {
		return "", errPatchEmailsValue
	}
	return normalizeEmail(chosen), nil
}
