package gossipcache

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type lifecycleConfig struct {
	value string
}

func TestServiceRegistryStartsAndShutsDownInOrder(t *testing.T) {
	cfg := &lifecycleConfig{value: "test"}
	var calls []string

	registry := NewServiceRegistry[lifecycleConfig]()
	for _, name := range []string{"logger", "metrics", "cache"} {
		name := name
		err := registry.Register(name, ServiceFunc[lifecycleConfig]{
			StartFunc: func(ctx context.Context, cfg *lifecycleConfig) error {
				if cfg.value != "test" {
					t.Fatalf("cfg.value = %q, want test", cfg.value)
				}
				calls = append(calls, "start:"+name)
				return nil
			},
			ShutdownFunc: func(ctx context.Context, cfg *lifecycleConfig) error {
				calls = append(calls, "shutdown:"+name)
				return nil
			},
		})
		if err != nil {
			t.Fatalf("Register(%q): %v", name, err)
		}
	}

	if err := registry.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := registry.Shutdown(context.Background(), cfg); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{
		"start:logger",
		"start:metrics",
		"start:cache",
		"shutdown:cache",
		"shutdown:metrics",
		"shutdown:logger",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestServiceRegistryShutsDownStartedServicesWhenStartFails(t *testing.T) {
	startErr := errors.New("bind port")
	var calls []string

	registry := NewServiceRegistry[lifecycleConfig]()
	mustRegister(t, registry, "first", ServiceFunc[lifecycleConfig]{
		StartFunc: func(context.Context, *lifecycleConfig) error {
			calls = append(calls, "start:first")
			return nil
		},
		ShutdownFunc: func(context.Context, *lifecycleConfig) error {
			calls = append(calls, "shutdown:first")
			return nil
		},
	})
	mustRegister(t, registry, "second", ServiceFunc[lifecycleConfig]{
		StartFunc: func(context.Context, *lifecycleConfig) error {
			calls = append(calls, "start:second")
			return startErr
		},
	})

	err := registry.Start(context.Background(), &lifecycleConfig{})
	if !errors.Is(err, startErr) {
		t.Fatalf("Start error = %v, want %v", err, startErr)
	}
	if !strings.Contains(err.Error(), `start service "second"`) {
		t.Fatalf("Start error = %q, want service name", err.Error())
	}

	want := []string{
		"start:first",
		"start:second",
		"shutdown:first",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestServiceRegistryReturnsShutdownErrors(t *testing.T) {
	shutdownErr := errors.New("flush failed")

	registry := NewServiceRegistry[lifecycleConfig]()
	mustRegister(t, registry, "cache", ServiceFunc[lifecycleConfig]{
		ShutdownFunc: func(context.Context, *lifecycleConfig) error {
			return shutdownErr
		},
	})

	if err := registry.Start(context.Background(), &lifecycleConfig{}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := registry.Shutdown(context.Background(), &lifecycleConfig{})
	if !errors.Is(err, shutdownErr) {
		t.Fatalf("Shutdown error = %v, want %v", err, shutdownErr)
	}
	if !strings.Contains(err.Error(), `shutdown service "cache"`) {
		t.Fatalf("Shutdown error = %q, want service name", err.Error())
	}
}

func TestServiceRegistryRejectsInvalidRegistration(t *testing.T) {
	registry := NewServiceRegistry[lifecycleConfig]()

	if err := registry.Register("", ServiceFunc[lifecycleConfig]{}); err == nil {
		t.Fatal("Register with empty name succeeded, want error")
	}
	if err := registry.Register("nil", nil); err == nil {
		t.Fatal("Register with nil service succeeded, want error")
	}
}

func mustRegister(t *testing.T, registry *ServiceRegistry[lifecycleConfig], name string, service Service[lifecycleConfig]) {
	t.Helper()
	if err := registry.Register(name, service); err != nil {
		t.Fatalf("Register(%q): %v", name, err)
	}
}
