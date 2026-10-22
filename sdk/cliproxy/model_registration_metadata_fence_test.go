package cliproxy

import (
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

func TestModelRegistrationRetriesMetadataOnlyVertexUpdate(t *testing.T) {
	for _, key := range []string{"openapi_models", "openapi-models", "models"} {
		t.Run(key, func(t *testing.T) {
			manager := coreauth.NewManager(nil, nil, nil)
			service := &Service{cfg: &config.Config{}, coreManager: manager}
			id := t.Name()
			reg := GlobalModelRegistry()
			registryEpoch := reg.ClientRegistrationEpoch(id)
			t.Cleanup(func() { reg.UnregisterClient(id) })
			interleaved := false
			runtime := &registrationInterleaveRuntime{onRead: func() {
				if interleaved {
					t.Fatal("obsolete runtime was reused on retry")
				}
				interleaved = true
				current, ok := manager.GetByID(id)
				if !ok {
					t.Fatal("auth missing")
				}
				epoch := current.RegistrationEpoch
				current.Metadata = map[string]any{key: []any{"qwen/new-model"}}
				current.Runtime = nil
				updated, err := manager.Update(t.Context(), current)
				if err != nil {
					t.Fatal(err)
				}
				if updated.RegistrationEpoch != epoch {
					t.Fatal("fixture unexpectedly changed registration identity")
				}
				if reg.ClientRegistrationEpoch(id) != registryEpoch {
					t.Fatal("fixture unexpectedly published another registry update")
				}
			}}
			auth, err := manager.Register(t.Context(), &coreauth.Auth{
				ID: id, Provider: "vertex", Status: coreauth.StatusActive, Runtime: runtime,
				Attributes: map[string]string{"auth_kind": "oauth"},
				Metadata:   map[string]any{key: []any{"qwen/old-model"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			service.completeModelRegistrationForAuthWithCache(t.Context(), auth, nil)
			if !interleaved {
				t.Fatal("metadata update did not interleave with registration")
			}
			if !reg.ClientSupportsModel(id, "qwen/new-model") || reg.ClientSupportsModel(id, "qwen/old-model") {
				t.Fatal("registration retained the superseded metadata catalog")
			}
		})
	}
}

func TestModelRegistrationFenceDetachesNestedMetadata(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{cfg: &config.Config{}, coreManager: manager}
	base, err := manager.Register(t.Context(), &coreauth.Auth{
		ID: t.Name(), Provider: "vertex", Status: coreauth.StatusActive,
		Metadata: map[string]any{"models": []any{map[string]any{"name": "qwen/old-model"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := GlobalModelRegistry()
	t.Cleanup(func() { reg.UnregisterClient(base.ID) })
	fence := newModelRegistrationFence(base, service.cfg)
	current, ok := manager.GetByID(base.ID)
	if !ok {
		t.Fatal("auth missing")
	}
	current.Metadata["models"].([]any)[0].(map[string]any)["name"] = "qwen/new-model"
	if _, errUpdate := manager.Update(t.Context(), current); errUpdate != nil {
		t.Fatal(errUpdate)
	}
	if fence.publishModels(service, base.ID, "vertex", []*ModelInfo{{ID: "qwen/old-model"}}) {
		t.Error("nested metadata mutation altered the captured publication fence")
	}
	pluginResult := base.Clone()
	pluginResult.Attributes = map[string]string{"plugin-result": "obsolete"}
	if _, committed, errUpdate := manager.UpdateIfRegistrationCurrent(t.Context(), base, pluginResult); errUpdate != nil || committed {
		t.Errorf("obsolete plugin auth update: committed=%v, error=%v", committed, errUpdate)
	}
}
