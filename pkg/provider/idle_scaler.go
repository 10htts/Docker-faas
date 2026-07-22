package provider

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

// ReclaimResult reports the resources released when a function's replicas are
// reclaimed to zero. It is proof of reclamation beyond "container stopped"
// (redteam objection SZ-09): container count, per-function network removal, and
// the memory/CPU capacity the removed containers had reserved.
type ReclaimResult struct {
	ContainersRemoved int
	NetworksRemoved   int
	MemoryBytesFreed  int64
	NanoCPUsFreed     int64
}

// ObservedReplicas returns the number of RUNNING containers the provider
// currently observes for a function. It reflects actual container state, not
// desired/stored replica count (SZ-09).
func (p *DockerProvider) ObservedReplicas(ctx context.Context, functionName string) (int, error) {
	containers, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return 0, err
	}
	running := 0
	for _, c := range containers {
		if isContainerRunningSummary(c) {
			running++
		}
	}
	return running, nil
}

// ownerID is this gateway deployment's stable ownership identity, used to scope
// orphan cleanup so it never touches another docker-faas instance's containers
// on a shared Docker daemon (RT-223). It is the FUNCTIONS_NETWORK name: stable
// across gateway restarts (so restart convergence still recognizes this
// gateway's own functions, SZ-07) and distinct per deployment.
func (p *DockerProvider) ownerID() string {
	return p.network
}

// ObservedFunctions returns the distinct function names the provider currently
// has containers for (running or not), SCOPED to containers this gateway owns.
// Used to detect orphan containers that no longer correspond to a declared
// function after a gateway restart (SZ-07). Containers owned by a different
// gateway (or carrying no ownership label — legacy/foreign) are ignored, so the
// idle reconciler can never reclaim another instance's functions (RT-223).
func (p *DockerProvider) ObservedFunctions(ctx context.Context) ([]string, error) {
	containers, err := p.listFunctionTypeContainers(ctx)
	if err != nil {
		return nil, err
	}
	owner := p.ownerID()
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, c := range containers {
		if c.Labels == nil {
			continue
		}
		// Only this gateway's own containers are eligible for orphan reclaim.
		if c.Labels[LabelGateway] != owner {
			continue
		}
		fn := c.Labels[LabelFunction]
		if fn == "" {
			continue
		}
		if _, ok := seen[fn]; ok {
			continue
		}
		seen[fn] = struct{}{}
		names = append(names, fn)
	}
	return names, nil
}

// ReclaimToZero stops and removes every container for a function, accounts the
// released memory/CPU, and cleans up the managed per-function network. It is
// idempotent: reclaiming an already-empty function returns a zero report. This
// is the provider-owned reclaim operation the idle reconciler invokes only
// after the generation/in-flight fence has been passed (SZ-01/SZ-09).
func (p *DockerProvider) ReclaimToZero(ctx context.Context, functionName string) (ReclaimResult, error) {
	var result ReclaimResult

	containers, err := p.listFunctionContainers(ctx, functionName)
	if err != nil {
		return result, err
	}

	networkName := ""
	for _, c := range containers {
		// Account the memory/CPU limits this container reserved before removal.
		if inspect, ierr := p.client.ContainerInspect(ctx, c.ID); ierr == nil {
			if inspect.HostConfig != nil {
				result.MemoryBytesFreed += inspect.HostConfig.Memory
				result.NanoCPUsFreed += inspect.HostConfig.NanoCPUs
			}
		} else {
			p.logger.Debugf("reclaim: inspect %s failed: %v", c.ID, ierr)
		}
		if networkName == "" && c.Labels != nil {
			networkName = c.Labels[LabelNetwork]
		}

		if err := p.removeContainerSummary(ctx, c); err != nil {
			return result, fmt.Errorf("failed to remove container during reclaim: %w", err)
		}
		result.ContainersRemoved++
	}

	// Clean up the managed per-function network so reclamation frees more than
	// just the containers (SZ-09).
	if networkName == "" {
		networkName = FunctionNetworkName(p.network, functionName)
	}
	if p.reclaimFunctionNetwork(ctx, functionName, networkName) {
		result.NetworksRemoved++
	}

	return result, nil
}

// reclaimFunctionNetwork removes a managed per-function network and reports
// whether it was actually removed. It never touches the shared base network.
func (p *DockerProvider) reclaimFunctionNetwork(ctx context.Context, functionName, networkName string) bool {
	if networkName == "" || networkName == p.network {
		return false
	}
	// Only report a removal for a network that actually existed when reclaim
	// began; otherwise an idempotent re-reclaim of an already-empty function
	// would fabricate NetworksRemoved=1 from the not-found confirmation below.
	if _, err := p.client.NetworkInspect(ctx, networkName, network.InspectOptions{}); err != nil {
		if !isNetworkNotFoundErr(err) {
			p.logger.Debugf("reclaim: inspect network %s: %v", networkName, err)
		}
		return false
	}
	if err := p.CleanupFunctionNetwork(ctx, functionName, networkName); err != nil {
		p.logger.Debugf("reclaim: cleanup network %s: %v", networkName, err)
		return false
	}
	// Confirm removal: if the network no longer exists it was reclaimed.
	if _, err := p.client.NetworkInspect(ctx, networkName, network.InspectOptions{}); err != nil {
		return isNetworkNotFoundErr(err)
	}
	return false
}

// listFunctionTypeContainers lists every function-type container OWNED by this
// gateway deployment. The gateway-ownership filter (RT-223) keeps orphan
// cleanup from ever enumerating a different docker-faas instance's containers
// on a shared Docker daemon.
func (p *DockerProvider) listFunctionTypeContainers(ctx context.Context) ([]container.Summary, error) {
	args := filters.NewArgs()
	args.Add("label", fmt.Sprintf("%s=function", LabelType))
	args.Add("label", fmt.Sprintf("%s=%s", LabelGateway, p.ownerID()))
	return p.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
}
