package luksd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/store"
	sdk "github.com/manchtools/cadestro/contract"
	pm "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const goodPassphrase = "correct-horse-battery-staple-42"

type fakeSession struct {
	mu             sync.Mutex
	order          *[]string
	validateResult *sdk.ValidateLuksTokenResult
	validateErr    error
	validateCalls  int
	keyResult      string
	keyErr         error
}

func (f *fakeSession) ValidateLuksToken(ctx context.Context, token string) (*sdk.ValidateLuksTokenResult, error) {
	f.mu.Lock()
	f.validateCalls++
	if f.order != nil {
		*f.order = append(*f.order, "validate")
	}
	f.mu.Unlock()
	if f.validateErr != nil {
		return nil, f.validateErr
	}
	return f.validateResult, nil
}

func (f *fakeSession) GetLuksKey(ctx context.Context, actionID string) (string, error) {
	f.mu.Lock()
	if f.order != nil {
		*f.order = append(*f.order, "getkey")
	}
	f.mu.Unlock()
	return f.keyResult, f.keyErr
}

type fakeStore struct {
	state       *store.LuksState
	stateErr    error
	hashes      []string
	setTypeArg  string
	addedHashes []string
}

func (f *fakeStore) GetLuksState(actionID string) (*store.LuksState, error) {
	return f.state, f.stateErr
}
func (f *fakeStore) GetLuksPassphraseHashes(actionID string) ([]string, error) {
	return f.hashes, nil
}
func (f *fakeStore) SetLuksDeviceKeyType(actionID, keyType string) error {
	f.setTypeArg = keyType
	return nil
}
func (f *fakeStore) AddLuksPassphraseHash(actionID, hash string) error {
	f.addedHashes = append(f.addedHashes, hash)
	return nil
}

type spyEnroller struct {
	order   *[]string
	addArgs []string
	kills   int
	wipes   int
	addErr  error
}

func (s *spyEnroller) AddKeyToSlot(ctx context.Context, devicePath string, slot int, unlockKey, newKey string) error {
	if s.order != nil {
		*s.order = append(*s.order, "addkey")
	}
	s.addArgs = append(s.addArgs, devicePath)
	return s.addErr
}
func (s *spyEnroller) KillSlot(ctx context.Context, devicePath string, slot int, unlockKey string) error {
	if s.order != nil {
		*s.order = append(*s.order, "killslot")
	}
	s.kills++
	return nil
}
func (s *spyEnroller) WipeTPM(ctx context.Context, devicePath, unlockKey string) error {
	if s.order != nil {
		*s.order = append(*s.order, "wipetpm")
	}
	s.wipes++
	return nil
}

func validResult() *sdk.ValidateLuksTokenResult {
	return &sdk.ValidateLuksTokenResult{
		ActionID:   "01HXLUKSDAEMON000000000000",
		DevicePath: "/dev/mapper/test",
		MinLength:  16,
		Complexity: pm.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_UNSPECIFIED,
	}
}

func TestLuksDaemon_RejectsRequestWithoutToken(t *testing.T) {
	t.Run("absent token", func(t *testing.T) {
		sess := &fakeSession{}
		enr := &spyEnroller{}
		d := NewDaemon("", &fakeStore{}, enr, nil)
		d.SetSession(sess)

		resp := d.handleRequest(context.Background(), Request{Token: "", Passphrase: goodPassphrase})
		assert.False(t, resp.OK)
		assert.Equal(t, CodeMissingToken, resp.Code)
		assert.Equal(t, 0, sess.validateCalls, "no token must not call the validator")
		assert.Empty(t, enr.addArgs, "no enrollment without a token")
	})

	t.Run("server-rejected token", func(t *testing.T) {
		sess := &fakeSession{validateErr: errors.New("token consumed")}
		enr := &spyEnroller{}
		d := NewDaemon("", &fakeStore{}, enr, nil)
		d.SetSession(sess)

		resp := d.handleRequest(context.Background(), Request{Token: "bogus26charstringnotvalidxx", Passphrase: goodPassphrase})
		assert.False(t, resp.OK)
		assert.Equal(t, CodeInvalidToken, resp.Code)
		assert.Empty(t, enr.addArgs, "a rejected token must not enroll")
	})

	t.Run("valid token validates before enrolling", func(t *testing.T) {
		var order []string
		sess := &fakeSession{order: &order, validateResult: validResult(), keyResult: "managed-key"}
		enr := &spyEnroller{order: &order}
		st := &fakeStore{state: &store.LuksState{DeviceKeyType: "none"}}
		d := NewDaemon("", st, enr, nil)
		d.SetSession(sess)

		resp := d.handleRequest(context.Background(), Request{Token: "validtoken", Passphrase: goodPassphrase})
		require.True(t, resp.OK, "valid request should succeed, got %+v", resp)
		require.NotEmpty(t, order)
		assert.Equal(t, "validate", order[0], "validation must precede any device mutation")
		assert.Contains(t, order, "addkey")
		vi, ai := indexOf(order, "validate"), indexOf(order, "addkey")
		assert.Less(t, vi, ai)
		assert.Equal(t, "user_passphrase", st.setTypeArg)
		assert.Len(t, st.addedHashes, 1, "passphrase hash recorded for reuse prevention")
	})
}

func TestLuksDaemon_RejectsWhenNotConnected(t *testing.T) {
	enr := &spyEnroller{}
	d := NewDaemon("", &fakeStore{}, enr, nil)
	resp := d.handleRequest(context.Background(), Request{Token: "t", Passphrase: goodPassphrase})
	assert.False(t, resp.OK)
	assert.Equal(t, CodeNotConnected, resp.Code)
	assert.Empty(t, enr.addArgs)
}

func TestLuksDaemon_TokenIsDeviceBoundAndSingleUse(t *testing.T) {
	for _, name := range []string{"wrong-device", "already-consumed", "expired"} {
		t.Run(name, func(t *testing.T) {
			sess := &fakeSession{validateErr: errors.New(name)}
			enr := &spyEnroller{}
			d := NewDaemon("", &fakeStore{}, enr, nil)
			d.SetSession(sess)
			resp := d.handleRequest(context.Background(), Request{Token: "tok", Passphrase: goodPassphrase})
			assert.False(t, resp.OK)
			assert.Equal(t, CodeInvalidToken, resp.Code)
			assert.Empty(t, enr.addArgs, "%s token must not enroll", name)
		})
	}
}

func TestLuksDaemon_NeverReadsDataDirOrCredentialStore(t *testing.T) {
	typ := reflect.TypeOf(Request{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name := strings.ToLower(f.Name + " " + f.Tag.Get("json"))
		assert.NotContains(t, name, "dir", "Request must not carry a data-dir/store-path field: %s", f.Name)
		assert.NotContains(t, name, "store", "Request must not carry a store-path field: %s", f.Name)
		assert.NotContains(t, name, "cred", "Request must not carry a credentials field: %s", f.Name)
	}

	var req Request
	dec := json.NewDecoder(strings.NewReader(`{"token":"t","passphrase":"p","data_dir":"/evil/store"}`))
	require.NoError(t, dec.Decode(&req))
	assert.Equal(t, "t", req.Token)
	assert.Equal(t, "p", req.Passphrase)
}

func TestLuksDaemon_PassphraseValidatedServerSide(t *testing.T) {
	sess := &fakeSession{validateResult: validResult(), keyResult: "managed-key"}
	enr := &spyEnroller{}
	d := NewDaemon("", &fakeStore{state: &store.LuksState{DeviceKeyType: "none"}}, enr, nil)
	d.SetSession(sess)

	resp := d.handleRequest(context.Background(), Request{Token: "tok", Passphrase: "short"})
	assert.False(t, resp.OK)
	assert.Equal(t, CodePassphrasePolicy, resp.Code)
	assert.Empty(t, enr.addArgs, "a policy-failing passphrase must not be enrolled")
}

func TestLuksDaemon_RevokesExistingKeyBeforeEnroll(t *testing.T) {
	var order []string
	sess := &fakeSession{order: &order, validateResult: validResult(), keyResult: "managed-key"}
	enr := &spyEnroller{order: &order}
	st := &fakeStore{state: &store.LuksState{DeviceKeyType: "tpm"}}
	d := NewDaemon("", st, enr, nil)
	d.SetSession(sess)

	resp := d.handleRequest(context.Background(), Request{Token: "tok", Passphrase: goodPassphrase})
	require.True(t, resp.OK, "%+v", resp)
	assert.Equal(t, 1, enr.wipes, "existing TPM key wiped")
	assert.Less(t, indexOf(order, "wipetpm"), indexOf(order, "addkey"), "revoke must precede enroll")
}

func TestLuksDaemon_RecordsEmptyKeyTypeWhenEnrollFailsAfterRevoke(t *testing.T) {
	sess := &fakeSession{validateResult: validResult(), keyResult: "managed-key"}
	enr := &spyEnroller{addErr: errors.New("cryptsetup: device busy")}
	st := &fakeStore{state: &store.LuksState{DeviceKeyType: "tpm"}}
	d := NewDaemon("", st, enr, nil)
	d.SetSession(sess)

	resp := d.handleRequest(context.Background(), Request{Token: "tok", Passphrase: goodPassphrase})
	assert.False(t, resp.OK)
	assert.Equal(t, CodeInternal, resp.Code)
	assert.Equal(t, 1, enr.wipes, "existing TPM key still revoked")
	assert.Equal(t, "none", st.setTypeArg, "store must not keep claiming a device key exists once the old one was revoked and re-enroll failed")
}

func TestLuksDaemon_SocketModeAndCleanup(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "luks.sock")
	require.NoError(t, os.WriteFile(sock, []byte("stale"), 0o600))

	d := NewDaemon(sock, &fakeStore{}, &spyEnroller{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Start(ctx) }()

	waitFor(t, func() bool {
		info, err := os.Stat(sock)
		return err == nil && info.Mode()&os.ModeSocket != 0
	})
	info, err := os.Stat(sock)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o622), info.Mode().Perm(),
		"socket must be write-only for group/other: connect(2) needs no read bit, and the peer-uid check — not the mode — is the authorization")

	cancel()
	<-done
	_, statErr := os.Stat(sock)
	assert.True(t, os.IsNotExist(statErr), "socket must be removed on shutdown")
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}
