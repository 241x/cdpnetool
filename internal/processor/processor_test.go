package processor_test

import (
	"context"
	"testing"
	"time"

	"cdpnetool/internal/auditor"
	"cdpnetool/internal/engine"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/processor"
	"cdpnetool/internal/tracker"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"
)

func TestNew(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())

	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())
	if p == nil {
		t.Error("New() returned nil")
	}
}

func TestProcessRequest_NoMatch(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	req := &domain.Request{
		ID:     "req1",
		URL:    "https://example.com",
		Method: "GET",
	}

	result := p.ProcessRequest(context.Background(), "test-session", "test-target", req)
	if result.Action != processor.ActionPass {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionPass)
	}
}

func TestProcessRequest_Block(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	// 添加拦截规则
	rule := rulespec.Rule{
		ID:      "rule1",
		Name:    "block rule",
		Enabled: true,
		Match: rulespec.Match{
			AllOf: []rulespec.Condition{
				{Type: rulespec.ConditionURLContains, Value: "example.com"},
			},
		},
		Actions: []rulespec.Action{
			{Type: rulespec.ActionBlock, StatusCode: 403, Body: "blocked"},
		},
		Stage: rulespec.StageRequest,
	}
	cfg.Rules = []rulespec.Rule{rule}
	eng.Update(cfg)

	req := &domain.Request{
		ID:     "req1",
		URL:    "https://example.com/test",
		Method: "GET",
	}

	result := p.ProcessRequest(context.Background(), "test-session", "test-target", req)
	if result.Action != processor.ActionBlock {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionBlock)
	}
	if result.MockRes == nil {
		t.Error("MockRes is nil")
	}
	if result.MockRes.StatusCode != 403 {
		t.Errorf("got status %v, want 403", result.MockRes.StatusCode)
	}
}

func TestProcessRequest_ModifyHeader(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	rule := rulespec.Rule{
		ID:      "rule1",
		Name:    "modify header",
		Enabled: true,
		Match: rulespec.Match{
			AllOf: []rulespec.Condition{
				{Type: rulespec.ConditionURLContains, Value: "example.com"},
			},
		},
		Actions: []rulespec.Action{
			{Type: rulespec.ActionSetHeader, Name: "X-Custom", Value: "test"},
		},
		Stage: rulespec.StageRequest,
	}
	cfg.Rules = []rulespec.Rule{rule}
	eng.Update(cfg)

	req := &domain.Request{
		ID:      "req1",
		URL:     "https://example.com/test",
		Method:  "GET",
		Headers: make(domain.Header),
	}

	result := p.ProcessRequest(context.Background(), "test-session", "test-target", req)
	if result.Action != processor.ActionModify {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionModify)
	}
	if result.ModifiedReq == nil {
		t.Error("ModifiedReq is nil")
	}
	if req.Headers.Get("X-Custom") != "test" {
		t.Errorf("got header %v, want test", req.Headers.Get("X-Custom"))
	}
}

func TestProcessRequest_ModifyURL(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	rule := rulespec.Rule{
		ID:      "rule1",
		Name:    "modify url",
		Enabled: true,
		Match: rulespec.Match{
			AllOf: []rulespec.Condition{
				{Type: rulespec.ConditionURLContains, Value: "old.com"},
			},
		},
		Actions: []rulespec.Action{
			{Type: rulespec.ActionSetUrl, Value: "https://new.com/path"},
		},
		Stage: rulespec.StageRequest,
	}
	cfg.Rules = []rulespec.Rule{rule}
	eng.Update(cfg)

	req := &domain.Request{
		ID:     "req1",
		URL:    "https://old.com/test",
		Method: "GET",
	}

	result := p.ProcessRequest(context.Background(), "test-session", "test-target", req)
	if result.Action != processor.ActionModify {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionModify)
	}
	if req.URL != "https://new.com/path" {
		t.Errorf("got url %v, want https://new.com/path", req.URL)
	}
}

func TestProcessResponse_NoMatch(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	result := p.ProcessResponse(context.Background(), "test-session", "test-target", "req1", &domain.Response{})
	if result.Action != processor.ActionPass {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionPass)
	}
}

func TestProcessResponse_ModifyStatus(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	rule := rulespec.Rule{
		ID:      "rule1",
		Name:    "modify status",
		Enabled: true,
		Match: rulespec.Match{
			AllOf: []rulespec.Condition{
				{Type: rulespec.ConditionURLContains, Value: "example.com"},
			},
		},
		Actions: []rulespec.Action{
			{Type: rulespec.ActionSetStatus, Value: float64(200)},
		},
		Stage: rulespec.StageResponse,
	}
	cfg.Rules = []rulespec.Rule{rule}
	eng.Update(cfg)

	req := &domain.Request{
		ID:     "req1",
		URL:    "https://example.com/test",
		Method: "GET",
	}

	// 先存入 tracker
	tr.Set("req1", &processor.PendingState{
		Request:      req,
		MatchedRules: nil,
		IsModified:   false,
	})

	res := &domain.Response{
		StatusCode: 404,
		Headers:    make(domain.Header),
	}

	result := p.ProcessResponse(context.Background(), "test-session", "test-target", "req1", res)
	if result.Action != processor.ActionModify {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionModify)
	}
	if res.StatusCode != 200 {
		t.Errorf("got status %v, want 200", res.StatusCode)
	}
}

func TestProcessResponse_ModifyHeader(t *testing.T) {
	tr := tracker.New(5*time.Second, logger.NewNop())
	defer tr.Stop()

	cfg := rulespec.NewConfig("test")
	eng := engine.New(cfg)

	events := make(chan domain.NetworkEvent, 10)
	trafficChan := make(chan domain.NetworkEvent, 10)
	matchedAud := auditor.New(events, logger.NewNop())
	trafficAud := auditor.New(trafficChan, logger.NewNop())
	p := processor.New(tr, eng, matchedAud, trafficAud, logger.NewNop())

	rule := rulespec.Rule{
		ID:      "rule1",
		Name:    "modify response header",
		Enabled: true,
		Match: rulespec.Match{
			AllOf: []rulespec.Condition{
				{Type: rulespec.ConditionURLContains, Value: "example.com"},
			},
		},
		Actions: []rulespec.Action{
			{Type: rulespec.ActionSetHeader, Name: "X-Response", Value: "modified"},
		},
		Stage: rulespec.StageResponse,
	}
	cfg.Rules = []rulespec.Rule{rule}
	eng.Update(cfg)

	req := &domain.Request{
		ID:     "req1",
		URL:    "https://example.com/test",
		Method: "GET",
	}

	tr.Set("req1", &processor.PendingState{
		Request:      req,
		MatchedRules: nil,
		IsModified:   false,
	})

	res := &domain.Response{
		StatusCode: 200,
		Headers:    make(domain.Header),
	}

	result := p.ProcessResponse(context.Background(), "test-session", "test-target", "req1", res)
	if result.Action != processor.ActionModify {
		t.Errorf("got action %v, want %v", result.Action, processor.ActionModify)
	}
	if res.Headers.Get("X-Response") != "modified" {
		t.Errorf("got header %v, want modified", res.Headers.Get("X-Response"))
	}
}

func TestPendingState_IsMatched(t *testing.T) {
	tests := []struct {
		name  string
		state *processor.PendingState
		want  bool
	}{
		{
			name: "无匹配规则",
			state: &processor.PendingState{
				MatchedRules: nil,
			},
			want: false,
		},
		{
			name: "有匹配规则",
			state: &processor.PendingState{
				MatchedRules: []*engine.MatchedRule{
					{Rule: &rulespec.Rule{ID: "rule1"}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsMatched()
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
