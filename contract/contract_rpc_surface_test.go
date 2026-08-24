package contract_test

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1" // registers the contract descriptors
)

func assertLiveFields[V any](t *testing.T, name string, fields map[protoreflect.FullName]V) {
	t.Helper()
	for fullName := range fields {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName)
		if err != nil {
			t.Errorf("%s entry %s resolves to no descriptor", name, fullName)
			continue
		}
		if _, ok := descriptor.(protoreflect.FieldDescriptor); !ok {
			t.Errorf("%s entry %s is not a field descriptor", name, fullName)
		}
	}
}

// The current golden guards the exact public contract. It is a target shape,
// never a comparison against an archived protocol.
const currentGoldenPath = "testdata/rpc_golden.json"

// The current contract must expose typed live control and no retired aliases.
// These assertions inspect the shipped descriptors directly.
func TestRPCSurface_CurrentTypedControl(t *testing.T) {
	live := liveSurface(t)
	methods := live["ControlService"]
	for _, name := range []string{"SyncDevice", "RebootDevice"} {
		if !contains(methods, name) {
			t.Errorf("MISSING current control RPC %s", name)
		}
	}
}

// TestRPCSurface_ProviderCapabilitiesArePublic holds the identity-provider
// capability shape that a login client reads before it offers a method. One
// OIDC client remains — the browser client — so client_id is the field every
// provider shape carries and browser_login is the capability derived from it.
func TestRPCSurface_ProviderCapabilitiesArePublic(t *testing.T) {
	for messageName, fields := range map[protoreflect.FullName]map[protoreflect.Name]protoreflect.Kind{
		"cadestro.v1.IdentityProvider": {
			"client_id": protoreflect.StringKind,
		},
		"cadestro.v1.CreateIdentityProviderRequest": {
			"client_id": protoreflect.StringKind,
		},
		"cadestro.v1.UpdateIdentityProviderRequest": {
			"client_id": protoreflect.StringKind,
		},
		"cadestro.v1.AuthMethodProvider": {
			"browser_login": protoreflect.BoolKind,
		},
	} {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(messageName)
		if err != nil {
			t.Fatalf("find %s: %v", messageName, err)
		}
		message := descriptor.(protoreflect.MessageDescriptor)
		for fieldName, wantKind := range fields {
			field := message.Fields().ByName(fieldName)
			if field == nil {
				t.Errorf("%s is missing field %s", messageName, fieldName)
				continue
			}
			if field.Kind() != wantKind {
				t.Errorf("%s.%s kind = %s, want %s", messageName, fieldName, field.Kind(), wantKind)
			}
		}
	}
}

type goldenSurface struct {
	Total    int                 `json:"total"`
	Services map[string][]string `json:"services"`
}

func loadGolden(t *testing.T, path string, minimumTotal, minimumServices int) goldenSurface {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var g goldenSurface
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	// Matches-zero: a golden that decayed to empty would make every assertion
	// below trivially true.
	if g.Total < minimumTotal || len(g.Services) < minimumServices {
		t.Fatalf("golden %s looks truncated (total=%d services=%d)", path, g.Total, len(g.Services))
	}
	// Self-consistency: `total` is recorded independently of the per-service
	// lists, so it is a second witness. Editing a name out of a list without
	// touching the total — the shape of a golden co-edited to match a mistaken
	// implementation change — fails here.
	sum := 0
	for _, v := range g.Services {
		sum += len(v)
	}
	if sum != g.Total {
		t.Fatalf("golden %s is internally inconsistent: total=%d but its service lists hold %d", path, g.Total, sum)
	}
	return g
}

// contractPackage is the protobuf namespace the contract ships under
// (target design §2). Every descriptor-level assertion in this file is scoped
// to it, so a stray descriptor from another module can never satisfy one.
const contractPackage = "cadestro.v1"

// liveSurface enumerates services and methods from the registered contract
// descriptors — the artifact that actually ships, not the .proto text.
func liveSurface(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if string(fd.Package()) != contractPackage {
			return true
		}
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			sd := svcs.Get(i)
			name := string(sd.Name())
			// Record the service key BEFORE walking methods. Recording it only
			// inside the method loop made a zero-method service invisible, so
			// `service GatewayService {}` would have satisfied both tests below
			// — the exact vacuous-pass this file exists to prevent.
			if _, ok := out[name]; !ok {
				out[name] = []string{}
			}
			ms := sd.Methods()
			for j := 0; j < ms.Len(); j++ {
				out[name] = append(out[name], string(ms.Get(j).Name()))
			}
		}
		return true
	})
	for k := range out {
		sort.Strings(out[k])
	}
	if len(out) == 0 {
		t.Fatalf("no %s services found in the descriptor registry — the enumeration is broken, "+
			"so a passing result would prove nothing", contractPackage)
	}
	return out
}

func assertSurfaceEqual(t *testing.T, got, want map[string][]string) {
	t.Helper()
	for svc, wantMethods := range want {
		gotMethods, ok := got[svc]
		if !ok {
			t.Errorf("MISSING service %s", svc)
			continue
		}
		for _, method := range wantMethods {
			if !contains(gotMethods, method) {
				t.Errorf("MISSING: %s/%s", svc, method)
			}
		}
	}
	for svc, gotMethods := range got {
		wantMethods, ok := want[svc]
		if !ok {
			t.Errorf("UNEXPECTED service %s", svc)
			continue
		}
		for _, method := range gotMethods {
			if !contains(wantMethods, method) {
				t.Errorf("UNEXPECTED: %s/%s", svc, method)
			}
		}
	}
}

func surfaceTotal(surface map[string][]string) int {
	total := 0
	for _, methods := range surface {
		total += len(methods)
	}
	return total
}

// TestRPCSurface_MatchesTargetGolden proves the deployed contract is exactly
// the reviewed target surface. The expected side never comes from the live
// descriptor, so an accidental deletion cannot disappear from both sides.
func TestRPCSurface_MatchesTargetGolden(t *testing.T) {
	want := loadGolden(t, currentGoldenPath, 150, 3)
	got := liveSurface(t)
	assertSurfaceEqual(t, got, want.Services)
	if total := surfaceTotal(got); total != want.Total {
		t.Errorf("RPC count: shipped %d, want %d", total, want.Total)
	}
}

// TestRPCSurface_SecretListsCannotLeakPlaintext holds the API boundary that
// makes each plaintext access independently auditable. Lists expose stable
// entry identifiers and metadata; only the one-entry reveal responses carry a
// secret value.
func TestRPCSurface_SecretListsCannotLeakPlaintext(t *testing.T) {
	for _, message := range []protoreflect.FullName{
		"cadestro.v1.LpsPassword",
		"cadestro.v1.LuksKey",
	} {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(message)
		if err != nil {
			t.Fatalf("find %s: %v", message, err)
		}
		fields := descriptor.(protoreflect.MessageDescriptor).Fields()
		if fields.ByName("id") == nil {
			t.Errorf("%s has no stable entry id", message)
		}
		for _, forbidden := range []protoreflect.Name{"password", "passphrase"} {
			if fields.ByName(forbidden) != nil {
				t.Errorf("%s metadata contains plaintext field %s", message, forbidden)
			}
		}
	}

	for message, secretField := range map[protoreflect.FullName]protoreflect.Name{
		"cadestro.v1.RevealLpsPasswordResponse": "password",
		"cadestro.v1.RevealLuksKeyResponse":     "passphrase",
	} {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(message)
		if err != nil {
			t.Fatalf("find %s: %v", message, err)
		}
		fields := descriptor.(protoreflect.MessageDescriptor).Fields()
		field := fields.ByName(secretField)
		if fields.Len() != 1 || field == nil || field.Kind() != protoreflect.StringKind {
			t.Errorf("%s must contain exactly one string field named %s", message, secretField)
		}
	}
}

// TestRPCSurface_GatewayServicesAreGone names the whole-service deletions
// separately: the enumeration above compares METHOD names, so a service
// stripped to zero methods raises nothing there — its (empty) method list
// matches an absent expectation vacuously. Every service whose entire method
// set spec 41 removes has to be named here or nothing checks it at all.
//
// InternalService was missing from this list, which is the failure the comment
// describes: internal.proto is deleted, and had it survived with its methods
// intact the enumeration would have caught it — but a partially-stripped one
// would have passed both.
func TestRPCSurface_GatewayServicesAreGone(t *testing.T) {
	live := liveSurface(t)
	for _, svc := range []string{"GatewayAuthService", "GatewayService", "InternalService"} {
		if methods, ok := live[svc]; ok {
			t.Errorf("service %s is still registered with %d method(s): %s",
				svc, len(methods), strings.Join(methods, ", "))
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Contract shape guard: the message-level properties the target design fixes.
// Subject is the registered descriptor set, i.e. what actually ships.
// ---------------------------------------------------------------------------

// contractMessages returns every message (nested included) in the namespace
// that declares AgentService. The namespace is discovered, not hardcoded, so
// the sweeps below keep scanning the real descriptors while the namespace
// itself is being renamed; TestContract_Namespace judges the name.
func contractMessages(t *testing.T) map[protoreflect.Name]protoreflect.MessageDescriptor {
	t.Helper()
	var pkg protoreflect.FullName
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if fd.Services().ByName("AgentService") == nil {
			return true
		}
		pkg = fd.Package()
		return false
	})
	if pkg == "" {
		t.Fatal("no registered file declares AgentService — the contract descriptors are not linked in")
	}
	out := map[protoreflect.Name]protoreflect.MessageDescriptor{}
	var walk func(protoreflect.MessageDescriptors)
	walk = func(mds protoreflect.MessageDescriptors) {
		for i := 0; i < mds.Len(); i++ {
			out[mds.Get(i).Name()] = mds.Get(i)
			walk(mds.Get(i).Messages())
		}
	}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if fd.Package() == pkg {
			walk(fd.Messages())
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("zero messages discovered in %s — the walk is broken", pkg)
	}
	return out
}

// Design §2: the contract lives under cadestro.v1. Both directions, so a
// rename that copied instead of moved (stale descriptors still registering at
// init) fails here.
// abandonedPackages are every namespace this contract has previously shipped
// under. Each rename ADDS to this list rather than replacing it: protoc leaves
// an orphaned .pb.go behind whenever a source file moves, and a .pb.go
// registers its descriptors at init, so a copied-instead-of-moved rename keeps
// the old namespace live in protoregistry while every source-level check
// reports clean. Dropping an older entry would retire that evidence.
var abandonedPackages = []string{"pm.v1", "powermanage.v1"}

func TestContract_Namespace(t *testing.T) {
	abandoned := map[string]bool{}
	for _, pkg := range abandonedPackages {
		if pkg == contractPackage {
			t.Fatalf("%s is listed as abandoned but is the shipped package — the guard would "+
				"contradict itself and can prove nothing", pkg)
		}
		abandoned[pkg] = true
	}

	var shipped, legacy []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		switch pkg := string(fd.Package()); {
		case pkg == contractPackage:
			shipped = append(shipped, fd.Path())
		case abandoned[pkg]:
			legacy = append(legacy, pkg+" ("+fd.Path()+")")
		}
		return true
	})
	if len(shipped) == 0 {
		t.Errorf("no descriptors registered under %s — the contract namespace has not moved", contractPackage)
	}
	if len(legacy) != 0 {
		sort.Strings(legacy)
		t.Errorf("stale descriptors from an abandoned namespace still registered: %s", strings.Join(legacy, ", "))
	}
}

// The target manifest and durable-result shape and §8
// (classified mTLS secrets),
// asserted by exact name and exact type.
func TestContract_TargetShape(t *testing.T) {
	msgs := contractMessages(t)

	for _, name := range []protoreflect.Name{
		"Manifest", "ManifestProvenance", "ManifestOccurrence", "ManifestResult",
	} {
		if _, ok := msgs[name]; !ok {
			t.Errorf("message %s is absent from the shipped contract", name)
		}
	}
	for _, f := range []struct {
		msg, field string
		kind       protoreflect.Kind
		msgType    protoreflect.Name // required when kind is MessageKind
		list       bool
		why        string
	}{
		{"Manifest", "manifest_id", protoreflect.StringKind, "", false, "the manifest has no identity"},
		{"Manifest", "provenance", protoreflect.MessageKind, "ManifestProvenance", false, "no bounded authoring provenance path"},
		{"Manifest", "occurrences", protoreflect.MessageKind, "ManifestOccurrence", true, "no ordered occurrence list"},
		{"ManifestOccurrence", "occurrence_id", protoreflect.StringKind, "", false, "authored positions are indistinguishable"},
		{"ManifestOccurrence", "action", protoreflect.MessageKind, "Action", false, "the occurrence carries no action"},
		{"ManifestOccurrence", "on_failure", protoreflect.EnumKind, "", false, "no per-occurrence failure policy"},
		{"ActionSet", "on_failure", protoreflect.EnumKind, "", false, "the set cannot retain its authored failure policy"},
		{"CreateActionSetRequest", "on_failure", protoreflect.EnumKind, "", false, "a set cannot be authored with STOP"},
		{"UpdateActionSetScheduleRequest", "on_failure", protoreflect.EnumKind, "", false, "the set execution policy cannot be changed"},
		{"ActionResult", "run_id", protoreflect.StringKind, "", false, "per-action result ingestion cannot be idempotent"},
		{"ActionResult", "occurrence_id", protoreflect.StringKind, "", false, "per-action result ingestion cannot be idempotent"},
		{"ManifestResult", "run_id", protoreflect.StringKind, "", false, "the manifest result cannot be matched to its run"},
		{"ManifestResult", "manifest_id", protoreflect.StringKind, "", false, "the manifest result names no manifest"},
		{"AgentMessage", "manifest_result", protoreflect.MessageKind, "ManifestResult", false, "there is no result for the complete manifest"},
	} {
		md, ok := msgs[protoreflect.Name(f.msg)]
		if !ok {
			t.Errorf("%s.%s: message %s is absent", f.msg, f.field, f.msg)
			continue
		}
		fd := md.Fields().ByName(protoreflect.Name(f.field))
		if fd == nil {
			t.Errorf("%s has no %s — %s", f.msg, f.field, f.why)
			continue
		}
		if fd.Kind() != f.kind {
			t.Errorf("%s.%s is %s, want %s", f.msg, f.field, fd.Kind(), f.kind)
			continue
		}
		if f.kind == protoreflect.MessageKind && fd.Message().Name() != f.msgType {
			t.Errorf("%s.%s carries %s, want %s", f.msg, f.field, fd.Message().Name(), f.msgType)
		}
		if fd.IsList() != f.list {
			t.Errorf("%s.%s repeated = %v, want %v", f.msg, f.field, fd.IsList(), f.list)
		}
	}

	// §7.2: a crash after a persisted STARTED reports INDETERMINATE instead of
	// silently re-running. The enum is reached through the field that uses it.
	if status := msgs["ActionResult"].Fields().ByName("status"); status == nil || status.Enum() == nil {
		t.Error("ActionResult has no enum-typed status field")
	} else if status.Enum().Values().ByName("EXECUTION_STATUS_INDETERMINATE") == nil {
		t.Errorf("%s has no EXECUTION_STATUS_INDETERMINATE — a crash after STARTED has no honest terminal status",
			status.Enum().FullName())
	}
}

// Design §3 requires exactly one agent-control transport. This exact-set check
// prevents a future convenience RPC from silently reintroducing a second path.
func TestContract_AgentServiceIsOneStream(t *testing.T) {
	live := liveSurface(t)
	methods, ok := live["AgentService"]
	if !ok {
		t.Fatal("AgentService is absent")
	}
	if len(methods) != 1 || methods[0] != "Stream" {
		t.Fatalf("AgentService methods = %v, want exactly [Stream]", methods)
	}
}

// Design §9–§10: single-implementation details are not public selectors.
// Speculative backend fields become selectable product surface even when no
// runtime implements the alternatives.
func TestContract_HasNoSpeculativeBackendSelectors(t *testing.T) {
	msgs := contractMessages(t)
	for _, name := range []protoreflect.Name{"ServiceParams", "EncryptionParams", "WifiParams"} {
		message, ok := msgs[name]
		if !ok {
			t.Errorf("message %s is absent", name)
			continue
		}
		if field := message.Fields().ByName("backend"); field != nil {
			t.Errorf("%s.backend still ships as a speculative selector", name)
		}
	}
	for _, enumName := range []protoreflect.Name{
		"ServiceBackend", "EncryptionBackend", "WifiBackend",
		"FirewallBackend", "DnsBackend", "NetworkConfigBackend",
	} {
		found := false
		protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
			if string(fd.Package()) == contractPackage && fd.Enums().ByName(enumName) != nil {
				found = true
				return false
			}
			return true
		})
		if found {
			t.Errorf("enum %s still ships without multiple implemented backends", enumName)
		}
	}
}

// Design §8: every field classified secret uses raw bytes on the authenticated
// mTLS stream, and no application frame carries a signature or the relay-era
// device-binding guard. Both are registry sweeps rather than lists — a NEW
// secret or a NEW signature field fails without anyone remembering to extend
// anything.
func TestContract_SecretsAreClassifiedAndFramesAreUnsigned(t *testing.T) {
	msgs := contractMessages(t)
	// These are the only plaintext secret fields in the contract: authenticated
	// HTTPS write-only inputs consumed and encrypted by control. They never enter
	// an agent-facing frame and no response message contains them.
	writeOnlyInputs := map[protoreflect.FullName]struct{}{
		"cadestro.v1.EncryptionAuthoringParams.preshared_key": {},
		"cadestro.v1.WifiAuthoringParams.psk":                 {},
		"cadestro.v1.WifiAuthoringParams.client_key":          {},
	}
	banned := map[protoreflect.Name]string{
		"signature":          "a CA signature over an application frame",
		"signed_envelope":    "the signed-envelope indirection",
		"target_device_id":   "the relay-era device-binding guard (mTLS identifies the device)",
		"standalone_actions": "the abolished pull-path scheduler shape (deliveries carry manifests)",
		"grouped_actions":    "the abolished pull-path scheduler shape (deliveries carry manifests)",
	}

	classified, scanned := 0, 0
	for _, md := range msgs {
		for i := 0; i < md.Fields().Len(); i++ {
			fd := md.Fields().Get(i)
			scanned++
			if why, bad := banned[fd.Name()]; bad {
				t.Errorf("%s.%s still ships — %s has no place on a direct mTLS transport", md.Name(), fd.Name(), why)
			}
			opts, _ := fd.Options().(*descriptorpb.FieldOptions)
			if !opts.GetDebugRedact() {
				continue
			}
			classified++
			if _, allowed := writeOnlyInputs[fd.FullName()]; allowed {
				continue
			}
			if fd.Kind() != protoreflect.BytesKind {
				t.Errorf("%s.%s is classified secret but ships as %s — it must be raw bytes on mTLS",
					md.Name(), fd.Name(), fd.Kind())
			}
		}
	}
	assertLiveFields(t, "writeOnlyInputs", writeOnlyInputs)
	if scanned == 0 {
		t.Fatal("matches-zero: scanned zero fields — the signing sweep proved nothing")
	}
	if classified == 0 {
		t.Fatal("matches-zero: no field carries the secret classification (debug_redact) — " +
			"the marker convention was dropped, so the classification sweep proved nothing")
	}
}

// Design §8: credentials authored by an operator are never ordinary strings
// in the Action message delivered to an agent. These exact fields were the gap
// that the general classification sweep could not see while they were left
// unclassified, so pin both their classification and their wire type.
func TestContract_ActionCredentialsAreDirectBytes(t *testing.T) {
	messages := contractMessages(t)
	for messageName, fieldNames := range map[protoreflect.Name][]protoreflect.Name{
		"EncryptionParams": {"preshared_key"},
		"WifiParams":       {"psk", "client_key"},
	} {
		message, ok := messages[messageName]
		if !ok {
			t.Errorf("message %s is absent", messageName)
			continue
		}
		for _, fieldName := range fieldNames {
			field := message.Fields().ByName(fieldName)
			if field == nil {
				t.Errorf("%s.%s is absent", messageName, fieldName)
				continue
			}
			options, _ := field.Options().(*descriptorpb.FieldOptions)
			if !options.GetDebugRedact() {
				t.Errorf("%s.%s is not classified with debug_redact", messageName, fieldName)
			}
			if field.Kind() != protoreflect.BytesKind {
				t.Errorf("%s.%s ships as %s, want bytes", messageName, fieldName, field.Kind())
			}
		}
	}
}

// Design §8: classification cannot be the only source of truth. A secret-like
// name that lacks the marker would otherwise be invisible to every downstream
// redaction/sealing guard at once. Metadata and deliberately plaintext public
// API fields must be named explicitly here with a narrow justification.
//
// An entry retires only when its FIELD is gone: assertLiveFields below fails on
// a justification that resolves to no descriptor, so an entry cannot be dropped
// to silence a finding while the field still ships. The message-level guard in
// TestContract_RetractedMessagesAreGone covers the other direction — a whole
// message deleted with the RPC that carried it must not return, because its
// secret-shaped fields would come back with no justification and no marker.
func TestContract_SecretShapedFieldsAreClassifiedOrJustified(t *testing.T) {
	secretName := regexp.MustCompile(`(?i)(token|secret|hmac|signature|fingerprint|password|passwd|digest|apikey|psk|private_key|preshared_key|client_key)`)
	metadataSuffixes := []string{
		"type", "kind", "id", "name", "len", "length", "count", "version",
		"expiry", "expiresat", "at", "format", "algorithm", "algo", "method",
		"status", "enabled", "disabled", "index", "idx", "field", "size", "configured",
		"url", "pin", "pagetoken", "nextpagetoken",
	}
	justifiedPlaintext := map[protoreflect.FullName]string{
		"cadestro.v1.EnableSCIMResponse.token":                    "one-time SCIM bearer reveal",
		"cadestro.v1.StartTerminalResponse.session_token":         "short-lived browser bearer output",
		"cadestro.v1.RegisterRequest.token":                       "one-time enrollment input",
		"cadestro.v1.RotateSCIMTokenResponse.token":               "one-time SCIM bearer reveal",
		"cadestro.v1.ValidateLuksTokenRequest.token":              "one-time device-bound LUKS input",
		"cadestro.v1.CreateTokenResponse.token":                   "one-time registration-token reveal",
		"cadestro.v1.CreateLuksTokenResponse.token":               "one-time LUKS-token reveal",
		"cadestro.v1.Hello.auth_token":                            "short-lived direct-stream bootstrap bearer",
		"cadestro.v1.RefreshTokenRequest.refresh_token":           "public HTTPS authentication input",
		"cadestro.v1.LogoutRequest.refresh_token":                 "public HTTPS authentication input",
		"cadestro.v1.EnrollRequest.token":                         "local privileged enrollment input",
		"cadestro.v1.CreateIdentityProviderRequest.client_secret": "authenticated HTTPS write-only input",
		"cadestro.v1.UpdateIdentityProviderRequest.client_secret": "authenticated HTTPS write-only input",
		"cadestro.v1.RevealLpsPasswordResponse.password":          "explicit audited operator reveal",
		"cadestro.v1.SSOCallbackResponse.access_token":            "public HTTPS authentication output",
		"cadestro.v1.SSOCallbackResponse.refresh_token":           "public HTTPS authentication output",
		"cadestro.v1.RefreshTokenResponse.access_token":           "public HTTPS authentication output",
		"cadestro.v1.RefreshTokenResponse.refresh_token":          "public HTTPS authentication output",
	}

	matches := 0
	for _, message := range contractMessages(t) {
		for i := 0; i < message.Fields().Len(); i++ {
			field := message.Fields().Get(i)
			if field.Kind() != protoreflect.StringKind && field.Kind() != protoreflect.BytesKind {
				continue
			}
			if !secretName.MatchString(string(field.Name())) {
				continue
			}
			normalized := strings.ReplaceAll(strings.ToLower(string(field.Name())), "_", "")
			metadata := false
			for _, suffix := range metadataSuffixes {
				if strings.HasSuffix(normalized, suffix) {
					metadata = true
					break
				}
			}
			if metadata {
				continue
			}
			matches++
			options, _ := field.Options().(*descriptorpb.FieldOptions)
			if options.GetDebugRedact() {
				continue
			}
			if _, allowed := justifiedPlaintext[field.FullName()]; !allowed {
				t.Errorf("%s looks secret but is neither classified nor justified", field.FullName())
			}
		}
	}
	assertLiveFields(t, "justifiedPlaintext", justifiedPlaintext)
	if matches == 0 {
		t.Fatal("matches-zero: no secret-shaped fields found; inverse classification guard proved nothing")
	}
}
