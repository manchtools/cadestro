package store

import (
	"context"
	"testing"
)

var ttyTestContext = context.Background()

func TestIsTTYEnabled_AcceptListExactBothDirections(t *testing.T) {
	enable := []string{"1", "true", "TRUE", " enabled ", "yes", "On"}
	disable := []string{"0", "", "garbage", "2", "disabled", "tru", "enabled!"}

	for _, v := range enable {
		t.Run("enable/"+v, func(t *testing.T) {
			st, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if err := st.SetSetting(ttyTestContext, TTYSettingKey, v); err != nil {
				t.Fatal(err)
			}
			enabled, err := st.IsTTYEnabled(ttyTestContext)
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				t.Errorf("IsTTYEnabled() with stored %q = false, want true", v)
			}
		})
	}

	for _, v := range disable {
		t.Run("disable/"+v, func(t *testing.T) {
			st, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if err := st.SetSetting(ttyTestContext, TTYSettingKey, v); err != nil {
				t.Fatal(err)
			}
			enabled, err := st.IsTTYEnabled(ttyTestContext)
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				t.Errorf("IsTTYEnabled() with stored %q = true, want false", v)
			}
		})
	}
}

func TestTTYDefault_Disabled(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	enabled, err := st.IsTTYEnabled(ttyTestContext)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("expected TTY to be disabled by default")
	}
}

func TestTTY_EnableDisableRoundtrip(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.SetTTYEnabled(ttyTestContext, true); err != nil {
		t.Fatal(err)
	}
	enabled, err := st.IsTTYEnabled(ttyTestContext)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Error("expected enabled after SetTTYEnabled(true)")
	}

	if err := st.SetTTYEnabled(ttyTestContext, true); err != nil {
		t.Fatal(err)
	}
	enabled, _ = st.IsTTYEnabled(ttyTestContext)
	if !enabled {
		t.Error("expected enabled after second SetTTYEnabled(true)")
	}

	if err := st.SetTTYEnabled(ttyTestContext, false); err != nil {
		t.Fatal(err)
	}
	enabled, err = st.IsTTYEnabled(ttyTestContext)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("expected disabled after SetTTYEnabled(false)")
	}

	if err := st.SetTTYEnabled(ttyTestContext, false); err != nil {
		t.Fatal(err)
	}
	enabled, _ = st.IsTTYEnabled(ttyTestContext)
	if enabled {
		t.Error("expected still disabled after second SetTTYEnabled(false)")
	}
}

func TestTTY_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetTTYEnabled(ttyTestContext, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	enabled, err := st2.IsTTYEnabled(ttyTestContext)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Error("expected TTY state to persist across reopen")
	}
}

func TestSettings_GetMissingReturnsEmpty(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	value, err := st.GetSetting(ttyTestContext, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if value != "" {
		t.Errorf("expected empty string for missing setting, got %q", value)
	}
}

func TestSettings_SetOverwrites(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.SetSetting(ttyTestContext, "key", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ttyTestContext, "key", "v2"); err != nil {
		t.Fatal(err)
	}

	value, _ := st.GetSetting(ttyTestContext, "key")
	if value != "v2" {
		t.Errorf("expected %q, got %q", "v2", value)
	}
}
