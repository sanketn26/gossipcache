package gossipcache

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Service is the lifecycle contract for components that need coordinated
// startup and graceful shutdown.
type Service[C any] interface {
	Start(ctx context.Context, cfg *C) error
	Shutdown(ctx context.Context, cfg *C) error
}

// ServiceFunc adapts start and shutdown functions to Service.
type ServiceFunc[C any] struct {
	StartFunc    func(context.Context, *C) error
	ShutdownFunc func(context.Context, *C) error
}

func (s ServiceFunc[C]) Start(ctx context.Context, cfg *C) error {
	if s.StartFunc == nil {
		return nil
	}
	return s.StartFunc(ctx, cfg)
}

func (s ServiceFunc[C]) Shutdown(ctx context.Context, cfg *C) error {
	if s.ShutdownFunc == nil {
		return nil
	}
	return s.ShutdownFunc(ctx, cfg)
}

// ServiceRegistry starts registered services in order and shuts them down in
// reverse order.
type ServiceRegistry[C any] struct {
	mu       sync.Mutex
	services []registeredService[C]
	started  []registeredService[C]
	running  bool
}

type registeredService[C any] struct {
	name    string
	service Service[C]
}

// NewServiceRegistry creates a registry for coordinating service lifecycles.
func NewServiceRegistry[C any]() *ServiceRegistry[C] {
	return &ServiceRegistry[C]{}
}

// Register adds a service to the lifecycle. Services are started in the order
// they are registered.
func (r *ServiceRegistry[C]) Register(name string, service Service[C]) error {
	if service == nil {
		return errors.New("service cannot be nil")
	}
	if name == "" {
		return errors.New("service name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return errors.New("cannot register service after start")
	}

	r.services = append(r.services, registeredService[C]{
		name:    name,
		service: service,
	})
	return nil
}

// Start starts all registered services. If a service fails to start, services
// that already started are shut down before Start returns.
func (r *ServiceRegistry[C]) Start(ctx context.Context, cfg *C) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return errors.New("service registry already started")
	}

	for _, item := range r.services {
		if err := item.service.Start(ctx, cfg); err != nil {
			shutdownErr := r.shutdownStarted(ctx, cfg)
			r.started = nil
			if shutdownErr != nil {
				return errors.Join(
					fmt.Errorf("start service %q: %w", item.name, err),
					fmt.Errorf("shutdown started services: %w", shutdownErr),
				)
			}
			return fmt.Errorf("start service %q: %w", item.name, err)
		}
		r.started = append(r.started, item)
	}

	r.running = true
	return nil
}

// Shutdown stops started services in reverse order.
func (r *ServiceRegistry[C]) Shutdown(ctx context.Context, cfg *C) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running && len(r.started) == 0 {
		return nil
	}

	err := r.shutdownStarted(ctx, cfg)
	r.started = nil
	r.running = false
	return err
}

func (r *ServiceRegistry[C]) shutdownStarted(ctx context.Context, cfg *C) error {
	var shutdownErr error
	for i := len(r.started) - 1; i >= 0; i-- {
		item := r.started[i]
		if err := item.service.Shutdown(ctx, cfg); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("shutdown service %q: %w", item.name, err),
			)
		}
	}
	return shutdownErr
}
