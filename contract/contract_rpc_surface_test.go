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

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
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

const currentGoldenPath = "testdata/rpc_golden.json"

func TestRPCSurface_CurrentTypedControl(t *testing.T) {
	live := liveSurface(t)
	methods := live["ControlService"]
	for _, name := range []string{"SyncDevice", "RebootDevice"} {
		if !contains(methods, name) {
			t.Errorf("MISSING current control RPC %s", name)
		}
	}
}

func TestRPCSurface_ProviderCapabilitiesArePublic(t *testing.T) {
	for messageName, fields := range map[protoreflect.FullName]map[protoreflect.Name]protoreflect.Kind{
		"cadestro.v1.IdentityProvider": {
			"client_id": protoreflect.MessageKind,
		},
		"cadestro.v1.CreateIdentityProviderRequest": {
			"client_id": protoreflect.MessageKind,
		},
		"cadestro.v1.UpdateIdentityProviderRequest": {
			"client_id": protoreflect.MessageKind,
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

	if g.Total < minimumTotal || len(g.Services) < minimumServices {
		t.Fatalf("golden %s looks truncated (total=%d services=%d)", path, g.Total, len(g.Services))
	}

	sum := 0
	for _, v := range g.Services {
		sum += len(v)
	}
	if sum != g.Total {
		t.Fatalf("golden %s is internally inconsistent: total=%d but its service lists hold %d", path, g.Total, sum)
	}
	return g
}

const contractPackage = "cadestro.v1"

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

func TestRPCSurface_MatchesTargetGolden(t *testing.T) {
	want := loadGolden(t, currentGoldenPath, 150, 3)
	got := liveSurface(t)
	assertSurfaceEqual(t, got, want.Services)
	if total := surfaceTotal(got); total != want.Total {
		t.Errorf("RPC count: shipped %d, want %d", total, want.Total)
	}
}

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
		msgType    protoreflect.Name
		list       bool
		why        string
	}{
		{"Manifest", "manifest_id", protoreflect.MessageKind, "ManifestId", false, "the manifest has no identity"},
		{"Manifest", "provenance", protoreflect.MessageKind, "ManifestProvenance", false, "no bounded authoring provenance path"},
		{"Manifest", "occurrences", protoreflect.MessageKind, "ManifestOccurrence", true, "no ordered occurrence list"},
		{"ManifestOccurrence", "occurrence_id", protoreflect.MessageKind, "OccurrenceId", false, "authored positions are indistinguishable"},
		{"ManifestOccurrence", "action", protoreflect.MessageKind, "Action", false, "the occurrence carries no action"},
		{"ManifestOccurrence", "on_failure", protoreflect.EnumKind, "", false, "no per-occurrence failure policy"},
		{"ActionSet", "on_failure", protoreflect.EnumKind, "", false, "the set cannot retain its authored failure policy"},
		{"CreateActionSetRequest", "on_failure", protoreflect.EnumKind, "", false, "a set cannot be authored with STOP"},
		{"UpdateActionSetScheduleRequest", "on_failure", protoreflect.EnumKind, "", false, "the set execution policy cannot be changed"},
		{"ActionResult", "run_id", protoreflect.MessageKind, "RunId", false, "per-action result ingestion cannot be idempotent"},
		{"ActionResult", "occurrence_id", protoreflect.MessageKind, "OccurrenceId", false, "per-action result ingestion cannot be idempotent"},
		{"ManifestResult", "run_id", protoreflect.MessageKind, "RunId", false, "the manifest result cannot be matched to its run"},
		{"ManifestResult", "manifest_id", protoreflect.MessageKind, "ManifestId", false, "the manifest result names no manifest"},
		{"OSQuery", "query_id", protoreflect.MessageKind, "QueryId", false, "an OS query has no identity"},
		{"OSQueryResult", "query_id", protoreflect.MessageKind, "QueryId", false, "an OS query result has no identity"},
		{"RequestInventory", "query_id", protoreflect.MessageKind, "QueryId", false, "an inventory request has no identity"},
		{"LogQuery", "query_id", protoreflect.MessageKind, "QueryId", false, "a log query has no identity"},
		{"LogQueryResult", "query_id", protoreflect.MessageKind, "QueryId", false, "a log query result has no identity"},
		{"DispatchOSQueryResponse", "query_id", protoreflect.MessageKind, "QueryId", false, "a dispatched OS query response has no identity"},
		{"GetOSQueryResultRequest", "query_id", protoreflect.MessageKind, "QueryId", false, "an OS query result request has no identity"},
		{"GetOSQueryResultResponse", "query_id", protoreflect.MessageKind, "QueryId", false, "an OS query result response has no identity"},
		{"QueryDeviceLogsResponse", "query_id", protoreflect.MessageKind, "QueryId", false, "a device log query response has no identity"},
		{"GetDeviceLogResultRequest", "query_id", protoreflect.MessageKind, "QueryId", false, "a device log result request has no identity"},
		{"GetDeviceLogResultResponse", "query_id", protoreflect.MessageKind, "QueryId", false, "a device log result response has no identity"},
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

	if status := msgs["ActionResult"].Fields().ByName("status"); status == nil || status.Enum() == nil {
		t.Error("ActionResult has no enum-typed status field")
	} else if status.Enum().Values().ByName("EXECUTION_STATUS_INDETERMINATE") == nil {
		t.Errorf("%s has no EXECUTION_STATUS_INDETERMINATE — a crash after STARTED has no honest terminal status",
			status.Enum().FullName())
	}
}

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

func TestContract_SecretsAreClassified(t *testing.T) {
	msgs := contractMessages(t)

	writeOnlyInputs := map[protoreflect.FullName]struct{}{
		"cadestro.v1.EncryptionAuthoringParams.preshared_key": {},
		"cadestro.v1.WifiAuthoringParams.psk":                 {},
		"cadestro.v1.WifiAuthoringParams.client_key":          {},
	}
	classified, scanned := 0, 0
	for _, md := range msgs {
		for i := 0; i < md.Fields().Len(); i++ {
			fd := md.Fields().Get(i)
			scanned++
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
