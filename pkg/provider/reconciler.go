package provider

import (
	"context"
	"sync"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

// ListNetworksFunc is a callback that returns the list of function network names.
type ListNetworksFunc func() ([]string, error)

// NetworkReconciler reconnects the gateway container to function networks.
type NetworkReconciler struct {
	client       *client.Client
	gatewayID    string
	logger       *logrus.Logger
	intervalSec  int
	listNetworks ListNetworksFunc

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewNetworkReconciler creates a new NetworkReconciler.
func NewNetworkReconciler(
	dockerClient *client.Client,
	gatewayID string,
	logger *logrus.Logger,
	intervalSeconds int,
	listNetworks ListNetworksFunc,
) *NetworkReconciler {
	return &NetworkReconciler{
		client:       dockerClient,
		gatewayID:    gatewayID,
		logger:       logger,
		intervalSec:  intervalSeconds,
		listNetworks: listNetworks,
		stopCh:       make(chan struct{}),
	}
}

// ReconcileOnce inspects the gateway container, lists function networks,
// and connects to any networks not already attached.
// Returns the number of networks attached and any error encountered.
func (r *NetworkReconciler) ReconcileOnce(ctx context.Context) (int, error) {
	if r.gatewayID == "" {
		return 0, nil
	}

	// Get current gateway networks
	inspect, err := r.client.ContainerInspect(ctx, r.gatewayID)
	if err != nil {
		return 0, err
	}

	attachedNetworks := make(map[string]bool)
	if inspect.NetworkSettings != nil {
		for netName := range inspect.NetworkSettings.Networks {
			attachedNetworks[netName] = true
		}
	}

	// Get function networks
	functionNetworks, err := r.listNetworks()
	if err != nil {
		return 0, err
	}

	// Connect to missing networks
	attachedCount := 0
	for _, netName := range functionNetworks {
		if netName == "" {
			continue
		}
		if attachedNetworks[netName] {
			continue
		}

		if err := r.connectToNetwork(ctx, netName); err != nil {
			if isNetworkNotFoundErr(err) {
				r.logger.Warnf("Network %s not found during reconciliation", netName)
				continue
			}
			r.logger.Errorf("Failed to connect gateway to network %s: %v", netName, err)
			continue
		}

		r.logger.Infof("Reconciliation: connected gateway to network %s", netName)
		attachedCount++
	}

	return attachedCount, nil
}

// connectToNetwork connects the gateway to the specified network.
func (r *NetworkReconciler) connectToNetwork(ctx context.Context, networkName string) error {
	err := r.client.NetworkConnect(ctx, networkName, r.gatewayID, &network.EndpointSettings{})
	if err != nil {
		if isAlreadyConnectedErr(err) {
			return nil
		}
		return err
	}
	return nil
}

// StartPeriodic starts a background goroutine that calls ReconcileOnce periodically.
func (r *NetworkReconciler) StartPeriodic(ctx context.Context) {
	if r.intervalSec <= 0 {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(time.Duration(r.intervalSec) * time.Second)
		defer ticker.Stop()

		r.logger.Infof("Network reconciliation started (interval: %ds)", r.intervalSec)

		for {
			select {
			case <-ctx.Done():
				r.logger.Info("Network reconciliation stopped (context cancelled)")
				return
			case <-r.stopCh:
				r.logger.Info("Network reconciliation stopped")
				return
			case <-ticker.C:
				attached, err := r.ReconcileOnce(ctx)
				if err != nil {
					r.logger.Errorf("Periodic network reconciliation failed: %v", err)
				} else if attached > 0 {
					r.logger.Infof("Periodic reconciliation: connected to %d networks", attached)
				}
			}
		}
	}()
}

// Stop terminates the periodic reconciliation loop.
func (r *NetworkReconciler) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}
