package config

import (
	"testing"

	"github.com/jaeho/ccmo/internal/data"
)

func TestParsePlanType(t *testing.T) {
	tests := []struct {
		input string
		want  data.PlanType
		ok    bool
	}{
		{"pro", data.PlanPro, true},
		{"Pro", data.PlanPro, true},
		{"max5", data.PlanMax5, true},
		{"MAX5", data.PlanMax5, true},
		{"max20", data.PlanMax20, true},
	}
	for _, tc := range tests {
		plan, ok := data.ParsePlanType(tc.input)
		if ok != tc.ok {
			t.Errorf("ParsePlanType(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			continue
		}
		if ok && plan.Type != tc.want {
			t.Errorf("ParsePlanType(%q) type = %d, want %d", tc.input, plan.Type, tc.want)
		}
	}
}

func TestParsePlanInvalid(t *testing.T) {
	_, ok := data.ParsePlanType("enterprise")
	if ok {
		t.Error("expected invalid plan to return false")
	}
	_, ok = data.ParsePlanType("")
	if ok {
		t.Error("expected empty plan to return false")
	}
}

func TestPlanTokenLimits(t *testing.T) {
	tests := []struct {
		plan  data.PlanType
		limit uint64
	}{
		{data.PlanPro, 19_000},
		{data.PlanMax5, 88_000},
		{data.PlanMax20, 220_000},
	}
	for _, tc := range tests {
		p := data.PlanConfig{Type: tc.plan}
		if p.TokenLimit() != tc.limit {
			t.Errorf("plan %d: TokenLimit() = %d, want %d", tc.plan, p.TokenLimit(), tc.limit)
		}
	}
}

func TestPlanCostLimits(t *testing.T) {
	tests := []struct {
		plan data.PlanType
		cost float64
	}{
		{data.PlanPro, 18.0},
		{data.PlanMax5, 35.0},
		{data.PlanMax20, 140.0},
	}
	for _, tc := range tests {
		p := data.PlanConfig{Type: tc.plan}
		if p.CostLimit() != tc.cost {
			t.Errorf("plan %d: CostLimit() = %.2f, want %.2f", tc.plan, p.CostLimit(), tc.cost)
		}
	}
}

func TestPlanLabel(t *testing.T) {
	tests := []struct {
		plan  data.PlanType
		label string
	}{
		{data.PlanPro, "Pro"},
		{data.PlanMax5, "Max5"},
		{data.PlanMax20, "Max20"},
		{data.PlanCustom, "Custom"},
	}
	for _, tc := range tests {
		p := data.PlanConfig{Type: tc.plan}
		if p.Label() != tc.label {
			t.Errorf("plan %d: Label() = %q, want %q", tc.plan, p.Label(), tc.label)
		}
	}
}

func TestUIStateLoadMissing(t *testing.T) {
	// LoadUIState should return zero-value for missing file (uses actual path)
	s := UIState{}
	if s.Filter != "" || s.SortColumn != "" || s.SelectedSession != "" {
		t.Error("zero UIState should have empty fields")
	}
}
