package contract_test

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func validateStruct(v protovalidate.Validator, msg proto.Message) (string, bool) {
	if err := v.Validate(msg); err != nil {
		return err.Error(), false
	}
	return "", true
}

func definedEnumValues[E ~int32](t *testing.T, names map[int32]string) []E {
	t.Helper()
	var defined []E
	for n := range names {
		defined = append(defined, E(n))
	}
	if len(defined) == 0 {
		t.Fatal("discovered zero defined enum values; the enumeration source moved")
	}
	return defined
}

func undefinedEnumValues[E ~int32](t *testing.T, names map[int32]string) []E {
	t.Helper()
	var out []E
	for _, n := range []int32{99, 3, -1, 2147483647} {
		if _, ok := names[n]; !ok {
			out = append(out, E(n))
		}
	}
	if len(out) == 0 {
		t.Fatal("every candidate out-of-range value is now a defined enum member; pick new ones")
	}
	return out
}

func mentionsField(detail, field string) bool { return strings.Contains(detail, field) }

func TestDefinedEnumLoopsRequireAValidFixture(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	p := encryptionParamsFixture()
	p.DeviceBoundKeyType = cadestrov1.EncryptionDeviceBoundKeyType_ENCRYPTION_DEVICE_BOUND_KEY_TYPE_TPM
	p.RotationIntervalDays = 0

	detail, ok := validateStruct(v, p)
	if ok {
		t.Fatalf("premise broken: rotation_interval_days = 0 must fail validation, got a pass (%s)", detail)
	}
	if !mentionsField(detail, "rotation_interval_days") {
		t.Errorf("expected the unrelated member to be blamed, got: %s", detail)
	}

	if mentionsField(detail, "device_bound_key_type") {
		t.Errorf("premise broken: a legal enum value must not be blamed, got: %s", detail)
	}
}

func encryptionParamsFixture() *cadestrov1.EncryptionParams {
	return &cadestrov1.EncryptionParams{
		PresharedKey:            make([]byte, 61),
		RotationIntervalDays:    30,
		MinWords:                5,
		UserPassphraseMinLength: 16,
	}
}

func encryptionAuthoringParamsFixture() *cadestrov1.EncryptionAuthoringParams {
	return &cadestrov1.EncryptionAuthoringParams{
		RotationIntervalDays:    30,
		MinWords:                5,
		UserPassphraseMinLength: 16,
	}
}

func TestEncryptionParams_RejectsUndefinedDeviceBoundKeyType(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	const field = "device_bound_key_type"

	for _, kt := range definedEnumValues[cadestrov1.EncryptionDeviceBoundKeyType](t, cadestrov1.EncryptionDeviceBoundKeyType_name) {
		p := encryptionParamsFixture()
		p.DeviceBoundKeyType = kt
		if detail, valid := validateStruct(v, p); !valid {
			t.Errorf("defined %s = %d (%s) must validate, got: %s", field, int32(kt), kt, detail)
		}
	}
	for _, kt := range undefinedEnumValues[cadestrov1.EncryptionDeviceBoundKeyType](t, cadestrov1.EncryptionDeviceBoundKeyType_name) {
		p := encryptionParamsFixture()
		p.DeviceBoundKeyType = kt
		detail, ok := validateStruct(v, p)
		if ok {
			t.Errorf("EncryptionParams with %s = %d passed validation; an out-of-range enum must be refused at the boundary, not silently degraded to 'no device-bound key' by the agent's switch default", field, int32(kt))
			continue
		}
		if !mentionsField(detail, field) {
			t.Errorf("%s = %d was rejected, but for another field: %s", field, int32(kt), detail)
		}
	}
}

func TestEncryptionAuthoringParams_RejectsUndefinedDeviceBoundKeyType(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	const field = "device_bound_key_type"

	for _, kt := range definedEnumValues[cadestrov1.EncryptionDeviceBoundKeyType](t, cadestrov1.EncryptionDeviceBoundKeyType_name) {
		p := encryptionAuthoringParamsFixture()
		p.DeviceBoundKeyType = kt
		if detail, valid := validateStruct(v, p); !valid {
			t.Errorf("defined %s = %d (%s) must validate, got: %s", field, int32(kt), kt, detail)
		}
	}
	for _, kt := range undefinedEnumValues[cadestrov1.EncryptionDeviceBoundKeyType](t, cadestrov1.EncryptionDeviceBoundKeyType_name) {
		p := encryptionAuthoringParamsFixture()
		p.DeviceBoundKeyType = kt
		detail, ok := validateStruct(v, p)
		if ok {
			t.Errorf("EncryptionAuthoringParams with %s = %d passed validation; the operator-facing write boundary must range-check the enum", field, int32(kt))
			continue
		}
		if !mentionsField(detail, field) {
			t.Errorf("%s = %d was rejected, but for another field: %s", field, int32(kt), detail)
		}
	}
}

func TestEncryptionParams_RejectsUndefinedUserPassphraseComplexity(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	const field = "user_passphrase_complexity"

	for _, c := range definedEnumValues[cadestrov1.LpsPasswordComplexity](t, cadestrov1.LpsPasswordComplexity_name) {
		p := encryptionParamsFixture()
		p.UserPassphraseComplexity = c
		if detail, valid := validateStruct(v, p); !valid {
			t.Errorf("defined %s = %d (%s) must validate, got: %s", field, int32(c), c, detail)
		}
	}
	for _, c := range undefinedEnumValues[cadestrov1.LpsPasswordComplexity](t, cadestrov1.LpsPasswordComplexity_name) {
		p := encryptionParamsFixture()
		p.UserPassphraseComplexity = c
		detail, ok := validateStruct(v, p)
		if ok {
			t.Errorf("EncryptionParams with %s = %d passed validation; an out-of-range enum must be refused at the boundary rather than resolving to the agent's default alphabet", field, int32(c))
			continue
		}
		if !mentionsField(detail, field) {
			t.Errorf("%s = %d was rejected, but for another field: %s", field, int32(c), detail)
		}
	}
}

func TestEncryptionAuthoringParams_RejectsUndefinedUserPassphraseComplexity(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	const field = "user_passphrase_complexity"

	for _, c := range definedEnumValues[cadestrov1.LpsPasswordComplexity](t, cadestrov1.LpsPasswordComplexity_name) {
		p := encryptionAuthoringParamsFixture()
		p.UserPassphraseComplexity = c
		if detail, valid := validateStruct(v, p); !valid {
			t.Errorf("defined %s = %d (%s) must validate, got: %s", field, int32(c), c, detail)
		}
	}
	for _, c := range undefinedEnumValues[cadestrov1.LpsPasswordComplexity](t, cadestrov1.LpsPasswordComplexity_name) {
		p := encryptionAuthoringParamsFixture()
		p.UserPassphraseComplexity = c
		detail, ok := validateStruct(v, p)
		if ok {
			t.Errorf("EncryptionAuthoringParams with %s = %d passed validation; the operator-facing write boundary must range-check the enum", field, int32(c))
			continue
		}
		if !mentionsField(detail, field) {
			t.Errorf("%s = %d was rejected, but for another field: %s", field, int32(c), detail)
		}
	}
}
