package live

import (
	"context"
	"encoding/json"
	"errors"
	agent "github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/history"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/workspace"
)

// This explicit opt-in permits one provider request through the native harness.
func TestSavedProviderLive(t *testing.T) {
	profile := os.Getenv("PLOUS_LIVE_AI_PROFILE")
	if profile == "" {
		t.Skip("set PLOUS_LIVE_AI_PROFILE to authorize one saved-provider request")
	}
	config, key := liveSavedConnection(t, profile)
	store, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	saved, err := history.Open(store)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := saved.NewConversation("", "Native provider smoke")
	if err != nil {
		t.Fatal(err)
	}
	const prompt = "Reply with exactly plous-live-ok. Do not use tools."
	turn, err := saved.StartTurn("", conversation.ID, history.Request{Agent: config, Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.DefaultRuntimeConfig()
	runtime.MaxSteps, runtime.TimeoutSeconds = 1, 90
	provider, err := aimodel.NewChatModel(t.Context(), config, key)
	if err != nil {
		t.Fatal(err)
	}
	limited := &singleRequestModel{base: provider, used: &atomic.Bool{}}
	t.Logf("Native smoke: provider=%s model=%s; one provider request maximum; temporary conversation", config.Provider, config.Model)
	result, err := agent.Run(t.Context(), agent.Options{Store: store, History: saved, ConversationID: conversation.ID, TurnID: turn.Request.ID, Config: config, Key: key,
		System: agent.Instructions + "\n" + config.Instructions, Prompt: prompt, Runtime: &runtime, Model: limited})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Text) != "plous-live-ok" || result.Finish != "stop" || !limited.used.Load() {
		t.Fatal("native harness did not complete the requested provider reply")
	}
	if err := saved.SaveResponse("", conversation.ID, turn.Request.ID, history.Response{Status: "completed", Text: result.Text}); err != nil {
		t.Fatal(err)
	}
	messages, err := saved.AgentMessages(t.Context(), "", conversation.ID)
	if err != nil || len(messages) != 2 || messages[1].Content != result.Text {
		t.Fatal("native exchange was not saved", err)
	}

}

func liveSavedConnection(t *testing.T, profile string) (aimodel.Config, string) {
	t.Helper()
	// Opening a Store can repair permissions. Read only a temporary settings copy.
	copyRoot := t.TempDir()
	if err := os.CopyFS(filepath.Join(copyRoot, "settings"), os.DirFS(filepath.Join(profile, "settings"))); err != nil {
		t.Fatal("could not copy saved settings for read-only validation")
	}
	store, err := workspace.Open(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	settings, err := store.ReadSettings()
	if err != nil {
		t.Fatal("saved settings failed validation")
	}
	config := settings.Agent
	key := os.Getenv(config.KeyEnv)
	if config.CredentialRef != "" {
		path := filepath.Join(profile, "credentials.json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatal("saved credentials must be a regular file under 1 MiB")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("could not read the saved credential")
		}
		var credentials map[string]string
		if json.Unmarshal(data, &credentials) != nil {
			t.Fatal("could not decode saved credentials")
		}
		key = credentials[config.CredentialRef]
	}
	if err := config.Ready(key); err != nil {
		t.Fatal(err)
	}
	return config, key
}

type singleRequestModel struct {
	base model.ToolCallingChatModel
	used *atomic.Bool
}

func (m *singleRequestModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.base.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &singleRequestModel{base: bound, used: m.used}, nil
}
func (m *singleRequestModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	if !m.used.CompareAndSwap(false, true) {
		return nil, errors.New("live smoke provider request limit reached")
	}
	return m.base.Generate(ctx, input, options...)
}
func (m *singleRequestModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if !m.used.CompareAndSwap(false, true) {
		return nil, errors.New("live smoke provider request limit reached")
	}
	return m.base.Stream(ctx, input, options...)
}

func TestLiveSmokeRequestLimitSurvivesToolBinding(t *testing.T) {
	script := &oneAnswerModel{}
	limited := &singleRequestModel{base: script, used: &atomic.Bool{}}
	bound, err := limited.WithTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Generate(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Stream(t.Context(), nil); err == nil {
		t.Fatal("a second provider request bypassed the shared limit")
	}
}

type oneAnswerModel struct{}

func (m *oneAnswerModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *oneAnswerModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "one request"}, nil
}
func (m *oneAnswerModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "one request"}}), nil
}
