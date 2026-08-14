package store

import "strings"

// TTYSettingKey is the settings row key storing the local TTY toggle state.
// Value is "1"/"true"/"enabled" (case-insensitive, whitespace-trimmed)
// when enabled; any other value (including absent) means disabled.
//
// The toggle is device-local — the database lives in the agent's
// root-owned, mode 0700 data directory, so only the agent process can
// write it and enable/disable is invoked via the cadestrod CLI, which
// requires sudo or equivalent escalation to root. The server cannot flip
// this flag directly; it can only request the flip via a shell action,
// which still runs on the device through that same CLI surface.
const TTYSettingKey = "tty.enabled"

// IsTTYEnabled returns true if remote terminal sessions are enabled on
// this device. Default (no row set) is false — terminals are off until
// a local admin explicitly enables them. Audit F059: accept the common
// truthy spellings (1/true/enabled, case-insensitive, whitespace-trimmed)
// instead of strict "1" only — the writer below still emits canonical
// "1"/"0", but a manual sqlite edit using "true" should not silently
// disable.
func (s *Store) IsTTYEnabled() (bool, error) {
	v, err := s.GetSetting(TTYSettingKey)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "enabled", "yes", "on":
		return true, nil
	}
	return false, nil
}

// SetTTYEnabled toggles the TTY state. Passing true enables, false disables.
func (s *Store) SetTTYEnabled(enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	return s.SetSetting(TTYSettingKey, v)
}
