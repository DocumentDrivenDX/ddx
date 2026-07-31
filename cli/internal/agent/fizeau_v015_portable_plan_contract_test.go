package agent

import (
	"context"
	"encoding/json"
	"fmt"
	reflect "reflect"
	"runtime"
	"strings"
	"testing"

	agentlib "github.com/easel/fizeau"
)

type portableRuntimeOpaquePlanService struct {
	called  int
	request agentlib.PortableRuntimeRequest
}

func (s *portableRuntimeOpaquePlanService) PreparePortableRuntime(_ context.Context, req agentlib.PortableRuntimeRequest) (*agentlib.PortableRuntimeBundle, error) {
	s.called++
	s.request = req
	return &agentlib.PortableRuntimeBundle{}, nil
}

func TestFizeauV015PortableRuntimePlanIsOpaqueAndRouteNeutral(t *testing.T) {
	t.Parallel()

	requestType := reflect.TypeOf(agentlib.PortableRuntimeRequest{})
	if requestType.PkgPath() != "github.com/easel/fizeau" || requestType.Name() != "PortableRuntimeRequest" {
		t.Fatalf("request identity = %s.%s", requestType.PkgPath(), requestType.Name())
	}
	assertExactPortableRuntimeFields(t, requestType, []portableRuntimeField{
		{"DestinationRoot", reflect.TypeFor[string]()},
		{"TargetGOOS", reflect.TypeFor[string]()},
		{"TargetGOARCH", reflect.TypeFor[string]()},
	})

	for _, forbidden := range []string{"Harness", "Provider", "Model", "Endpoint", "ServerInstance"} {
		if _, ok := requestType.FieldByName(forbidden); ok {
			t.Fatalf("portable runtime request exposes route field %s", forbidden)
		}
	}

	request := agentlib.PortableRuntimeRequest{
		DestinationRoot: "/private/runtime-root",
		TargetGOOS:      "linux",
		TargetGOARCH:    runtime.GOARCH,
	}
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	requestDiagnostics := fmt.Sprintf("%s %v %+v %#v", encodedRequest, request, request, request)
	if strings.Contains(requestDiagnostics, request.DestinationRoot) || strings.Contains(requestDiagnostics, "private/runtime-root") {
		t.Fatalf("request diagnostics expose DestinationRoot: %s", requestDiagnostics)
	}

	svc := &portableRuntimeOpaquePlanService{}
	bundle, err := svc.PreparePortableRuntime(context.Background(), request)
	if err != nil {
		t.Fatalf("PreparePortableRuntime: %v", err)
	}
	if svc.called != 1 {
		t.Fatalf("PreparePortableRuntime called %d times, want 1", svc.called)
	}
	if svc.request != request {
		t.Fatalf("request changed across pass-through: got %#v want %#v", svc.request, request)
	}

	bundleType := reflect.TypeOf(agentlib.PortableRuntimeBundle{})
	if bundleType.PkgPath() != "github.com/easel/fizeau" || bundleType.Name() != "PortableRuntimeBundle" {
		t.Fatalf("bundle identity = %s.%s", bundleType.PkgPath(), bundleType.Name())
	}
	for index := 0; index < bundleType.NumField(); index++ {
		if bundleType.Field(index).IsExported() {
			t.Fatalf("portable bundle exposes field %s", bundleType.Field(index).Name)
		}
	}
	requiredMethods := map[string]bool{
		"RuntimeRoot":      false,
		"Mounts":           false,
		"EnvironmentNames": false,
		"Close":            false,
	}
	pointerType := reflect.TypeOf(&agentlib.PortableRuntimeBundle{})
	for index := 0; index < pointerType.NumMethod(); index++ {
		method := pointerType.Method(index)
		if _, ok := requiredMethods[method.Name]; !ok {
			t.Fatalf("portable bundle exposes unexpected method %s", method.Name)
		}
		requiredMethods[method.Name] = true
	}
	for method, present := range requiredMethods {
		if !present {
			t.Fatalf("portable bundle lacks method %s", method)
		}
	}

	if bundle == nil {
		t.Fatal("PreparePortableRuntime returned nil bundle")
	}
	if bundle.RuntimeRoot() != "" || len(bundle.Mounts()) != 0 || len(bundle.EnvironmentNames()) != 0 {
		t.Fatal("zero portable runtime bundle is not opaque-empty")
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("bundle.Close(): %v", err)
	}
}

type portableRuntimeField struct {
	name   string
	typeOf reflect.Type
}

func assertExactPortableRuntimeFields(t *testing.T, typ reflect.Type, want []portableRuntimeField) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s field count = %d, want %d", typ.Name(), typ.NumField(), len(want))
	}
	for index, expected := range want {
		field := typ.Field(index)
		if field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("%s field %d = %s %v, want %s %v", typ.Name(), index, field.Name, field.Type, expected.name, expected.typeOf)
		}
	}
}
