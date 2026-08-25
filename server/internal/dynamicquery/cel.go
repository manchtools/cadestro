package dynamicquery

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
)

const (
	maxQueryLength = 4096
	maxCost        = 100000
)

type Device struct {
	Hostname        string            `cel:"hostname"`
	OS              string            `cel:"os"`
	OSVersion       string            `cel:"os_version"`
	OSMajor         int64             `cel:"os_major"`
	OSMinor         int64             `cel:"os_minor"`
	OSArch          string            `cel:"os_arch"`
	OSPlatform      string            `cel:"os_platform"`
	OSPlatformLike  string            `cel:"os_platform_like"`
	CPUType         string            `cel:"cpu_type"`
	CPUBrand        string            `cel:"cpu_brand"`
	CPUCores        int64             `cel:"cpu_cores"`
	CPULogicalCores int64             `cel:"cpu_logical_cores"`
	MemoryTotal     int64             `cel:"memory_total"`
	Kernel          string            `cel:"kernel"`
	Labels          map[string]string `cel:"labels"`
	Groups          []string          `cel:"groups"`
}

type User struct {
	Email             string `cel:"email"`
	Disabled          bool   `cel:"disabled"`
	DisplayName       string `cel:"display_name"`
	PreferredUsername string `cel:"preferred_username"`
	Locale            string `cel:"locale"`
}

type DeviceQuery struct{ program cel.Program }

type UserQuery struct{ program cel.Program }

type deviceActivation struct{ device Device }

func (a deviceActivation) ResolveName(name string) (any, bool) {
	if name == "device" {
		return a.device, true
	}
	return nil, false
}

func (deviceActivation) Parent() cel.Activation { return nil }

type userActivation struct{ user User }

func (a userActivation) ResolveName(name string) (any, bool) {
	if name == "user" {
		return a.user, true
	}
	return nil, false
}

func (userActivation) Parent() cel.Activation { return nil }

func CompileDevice(raw string) (DeviceQuery, error) {
	program, err := compile(raw, "device", "dynamicquery.Device", reflect.TypeFor[Device]())
	if err != nil {
		return DeviceQuery{}, fmt.Errorf("compile device query: %w", err)
	}
	return DeviceQuery{program: program}, nil
}

func CompileUser(raw string) (UserQuery, error) {
	program, err := compile(raw, "user", "dynamicquery.User", reflect.TypeFor[User]())
	if err != nil {
		return UserQuery{}, fmt.Errorf("compile user query: %w", err)
	}
	return UserQuery{program: program}, nil
}

func (q DeviceQuery) Eval(ctx context.Context, device Device) (bool, error) {
	return eval(ctx, q.program, deviceActivation{device: device})
}

func (q UserQuery) Eval(ctx context.Context, user User) (bool, error) {
	return eval(ctx, q.program, userActivation{user: user})
}

func compile(raw, variable, typeName string, nativeType reflect.Type) (cel.Program, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if len(raw) > maxQueryLength {
		return nil, fmt.Errorf("query exceeds %d bytes", maxQueryLength)
	}
	env, err := cel.NewEnv(
		cel.Variable(variable, cel.ObjectType(typeName)),
		ext.NativeTypes(nativeType, ext.ParseStructTag("cel")),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}
	ast, issues := env.Compile(raw)
	if issues.Err() != nil {
		return nil, fmt.Errorf("compile expression: %w", issues.Err())
	}
	if !ast.OutputType().IsEquivalentType(cel.BoolType) {
		return nil, fmt.Errorf("compile expression: result type %s is not bool", ast.OutputType())
	}
	program, err := env.Program(ast, cel.InterruptCheckFrequency(1000), cel.CostLimit(maxCost))
	if err != nil {
		return nil, fmt.Errorf("build CEL program: %w", err)
	}
	return program, nil
}

func eval(ctx context.Context, program cel.Program, activation cel.Activation) (bool, error) {
	if program == nil {
		return false, fmt.Errorf("query is not compiled")
	}
	if ctx == nil {
		return false, fmt.Errorf("evaluate query: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("evaluate query: %w", err)
	}
	out, _, err := program.ContextEval(ctx, activation)
	if err != nil {
		return false, fmt.Errorf("evaluate query: %w", err)
	}
	result, ok := out.(types.Bool)
	if !ok {
		return false, fmt.Errorf("evaluate query: result is %s, not bool", out.Type())
	}
	return bool(result), nil
}
