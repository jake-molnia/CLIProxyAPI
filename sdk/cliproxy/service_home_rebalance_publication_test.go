package cliproxy

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/home"
)

func TestRebalanceReportsSuccessOnlyAfterConfigPublication(t *testing.T) {
	for _, cancelBeforePublish := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelBeforePublish), func(t *testing.T) {
			source := newRebalanceAdmissionHome(t, false)
			target := newRebalanceAdmissionHome(t, false)
			source.mu.Lock()
			source.target = target.listener.Addr().String()
			source.mu.Unlock()
			forwarder := &rebalanceAdmissionForwarder{bound: make(chan *home.Client, 4), detached: make(chan *home.Client, 4)}
			previousStart := startHomeLogForwarder
			startHomeLogForwarder = func(int) homeLogForwarder { return forwarder }
			t.Cleanup(func() { startHomeLogForwarder = previousStart })
			service := newRegistryTestService(t, source.listener)
			service.cfg.Home.DisableClusterDiscovery = false
			ctx, cancel := context.WithCancel(context.Background())
			stageStarted, releaseStage := make(chan struct{}), make(chan struct{})
			var stages atomic.Int32
			service.homeConfigStageHook = func() {
				if stages.Add(1) == 2 {
					close(stageStarted)
					select {
					case <-releaseStage:
					case <-ctx.Done():
					}
				}
			}
			service.runCancel = cancel
			service.startHomeSubscriber(ctx)
			t.Cleanup(func() {
				cancel()
				awaitRebalanceEvent(t, service.homeSupervisor.done)
				home.ClearCurrent()
			})
			sourceClient := awaitRebalanceEvent(t, forwarder.bound)
			close(source.migrate)
			awaitRebalanceEvent(t, stageStarted)
			// The target has acknowledged its subscription, but its replacement
			// configuration is blocked before commit, apply, and publication.
			select {
			case result := <-target.results:
				t.Fatalf("rebalance reported before configuration publication: %v", result)
			default:
			}
			if cancelBeforePublish {
				cancel()
				awaitRebalanceEvent(t, service.homeSupervisor.done)
				select {
				case result := <-target.results:
					t.Fatalf("canceled publication reported a rebalance result: %v", result)
				default:
				}
				return
			}
			close(releaseStage)
			result := awaitRebalanceEvent(t, target.results)
			if len(result) != 5 || result[3] != "admission-plan" || result[4] != "success" {
				t.Fatalf("unexpected rebalance result: %v", result)
			}
			service.homeMu.Lock()
			client, registry := service.homeClient, service.homeRegistry
			service.homeMu.Unlock()
			if client == nil || client == sourceClient || registry == nil || !client.HeartbeatOK() {
				t.Fatal("success arrived before the replacement dispatch bundle was published")
			}
		})
	}
}
