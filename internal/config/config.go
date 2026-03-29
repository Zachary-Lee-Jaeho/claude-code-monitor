package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jaeho/ccmo/internal/data"
	"github.com/jaeho/ccmo/internal/security"
)

type configFile struct {
	Plan string `json:"plan"`
}

// LoadPlan loads the saved plan from ~/.ccmo/config.json.
func LoadPlan() (data.PlanConfig, bool) {
	raw, err := os.ReadFile(ConfigPath())
	if err != nil {
		return data.PlanConfig{}, false
	}
	var cfg configFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return data.PlanConfig{}, false
	}
	return data.ParsePlanType(cfg.Plan)
}

// SavePlan saves the plan to ~/.ccmo/config.json.
func SavePlan(plan data.PlanConfig) error {
	if err := security.EnsureDir(AppDir()); err != nil {
		return err
	}
	cfg := configFile{Plan: strings.ToLower(plan.Label())}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := ConfigPath()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return security.EnsureFilePermissions(path, 0o600)
}

// PromptPlanSelection interactively asks the user to choose a plan.
// Uses plain stdin (not TUI) for reliability.
func PromptPlanSelection() data.PlanConfig {
	fmt.Println()
	fmt.Println("  Welcome to CCMO — Claude Code Monitor")
	fmt.Println()
	fmt.Println("  Select your Claude Code plan:")
	fmt.Println()
	fmt.Println("    1. Pro      ($18/mo  — 19k output tokens/week)")
	fmt.Println("    2. Max 5    ($35/mo  — 88k output tokens/week)")
	fmt.Println("    3. Max 20   ($140/mo — 220k output tokens/week)")
	fmt.Println()
	fmt.Print("  Choice [1-3] (default 2): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var plan data.PlanConfig
	switch input {
	case "1":
		plan = data.PlanConfig{Type: data.PlanPro}
	case "3":
		plan = data.PlanConfig{Type: data.PlanMax20}
	default:
		plan = data.PlanConfig{Type: data.PlanMax5}
	}

	fmt.Printf("\n  Selected: %s\n", plan.Label())
	SavePlan(plan)
	return plan
}
