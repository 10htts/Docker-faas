package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestNewNetworkReconciler(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	listNetworks := func() ([]string, error) {
		return []string{"net1", "net2"}, nil
	}

	reconciler := NewNetworkReconciler(nil, "gateway123", logger, 30, listNetworks)

	if reconciler == nil {
		t.Fatal("expected reconciler to be created")
	}
	if reconciler.gatewayID != "gateway123" {
		t.Errorf("expected gatewayID gateway123, got %s", reconciler.gatewayID)
	}
	if reconciler.intervalSec != 30 {
		t.Errorf("expected intervalSec 30, got %d", reconciler.intervalSec)
	}
}

func TestReconcileOnceNoGateway(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	listNetworks := func() ([]string, error) {
		return []string{"net1"}, nil
	}

	reconciler := NewNetworkReconciler(nil, "", logger, 60, listNetworks)

	attached, err := reconciler.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attached != 0 {
		t.Errorf("expected 0 attached networks when no gateway, got %d", attached)
	}
}

func TestListNetworksFuncReturnsEmpty(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	called := false
	listNetworks := func() ([]string, error) {
		called = true
		return []string{}, nil
	}

	// With empty gateway ID, listNetworks should not be called
	reconciler := NewNetworkReconciler(nil, "", logger, 60, listNetworks)

	_, err := reconciler.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("listNetworks should not be called when gatewayID is empty")
	}
}

func TestStopReconciler(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	listNetworks := func() ([]string, error) {
		return []string{}, nil
	}

	reconciler := NewNetworkReconciler(nil, "", logger, 1, listNetworks)

	// Start periodic with empty gateway (should not start goroutine since gatewayID is empty)
	ctx := context.Background()
	reconciler.StartPeriodic(ctx)

	// Stop should not hang even without periodic running
	done := make(chan struct{})
	go func() {
		reconciler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestStartPeriodicWithZeroInterval(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	listNetworks := func() ([]string, error) {
		return []string{}, nil
	}

	reconciler := NewNetworkReconciler(nil, "gateway123", logger, 0, listNetworks)

	ctx := context.Background()
	reconciler.StartPeriodic(ctx)

	// Should not start any goroutine since interval is 0
	// Stop should complete immediately
	done := make(chan struct{})
	go func() {
		reconciler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Stop() timed out when interval is 0")
	}
}

func TestIsAlreadyConnectedErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"already exists", errors.New("endpoint already exists"), true},
		{"already connected", errors.New("container already connected"), true},
		{"other error", errors.New("network not found"), false},
		{"case insensitive", errors.New("ALREADY EXISTS in network"), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isAlreadyConnectedErr(tc.err)
			if result != tc.expected {
				t.Errorf("isAlreadyConnectedErr(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestIsNetworkNotFoundErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"network not found", errors.New("network xyz not found"), true},
		{"Network Not Found", errors.New("Network foo Not Found"), true},
		{"other error", errors.New("connection refused"), false},
		{"only network", errors.New("network issue"), false},
		{"only not found", errors.New("container not found"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isNetworkNotFoundErr(tc.err)
			if result != tc.expected {
				t.Errorf("isNetworkNotFoundErr(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}
