package act

import (
	"path/filepath"
	"testing"

	specyaml "github.com/bradrydzewski/spec/yaml"
)

func TestParseTestFixtures(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no test fixtures found")
	}

	for _, f := range fixtures {
		t.Run(filepath.Base(f), func(t *testing.T) {
			schema, err := specyaml.ParseFile(f)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			stages, err := resolveStages(schema)
			if err != nil {
				t.Fatalf("resolve stages: %v", err)
			}
			if len(stages) == 0 {
				t.Fatal("expected at least one stage")
			}
		})
	}
}

func TestTopologicalSort(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/stage_deps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)
	batches := topologicalSort(stages)

	if len(batches) < 3 {
		t.Fatalf("expected at least 3 batches, got %d", len(batches))
	}

	// First batch should be setup (no deps)
	if stageName(batches[0][0]) != "setup" {
		t.Errorf("expected first batch to be setup, got %q", stageName(batches[0][0]))
	}

	// Second batch should contain build and lint (both depend only on setup)
	if len(batches[1]) != 2 {
		t.Errorf("expected second batch to have 2 stages, got %d", len(batches[1]))
	}

	// Last batch should be deploy
	last := batches[len(batches)-1]
	if stageName(last[0]) != "deploy" {
		t.Errorf("expected last batch to be deploy, got %q", stageName(last[0]))
	}
}

func TestExpandMatrix(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)
	if stages[0].Strategy == nil || stages[0].Strategy.Matrix == nil {
		t.Fatal("expected matrix strategy")
	}

	combos := expandMatrix(stages[0].Strategy.Matrix)
	// 3 go versions * 2 os = 6 combinations
	if len(combos) != 6 {
		t.Fatalf("expected 6 matrix combinations, got %d", len(combos))
	}

	for _, c := range combos {
		if c["go_version"] == "" || c["os"] == "" {
			t.Errorf("incomplete combo: %v", c)
		}
	}
}

func TestEnvMerge(t *testing.T) {
	base := map[string]string{"A": "1", "B": "2"}
	overlay := map[string]string{"B": "override", "C": "3"}

	result := mergeEnv(base, overlay)
	if result["A"] != "1" {
		t.Errorf("expected A=1, got %q", result["A"])
	}
	if result["B"] != "override" {
		t.Errorf("expected B=override, got %q", result["B"])
	}
	if result["C"] != "3" {
		t.Errorf("expected C=3, got %q", result["C"])
	}
}

func TestFilterStage(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/stage_deps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)

	filtered := filterStage(stages, "build")
	if filtered == nil {
		t.Fatal("expected to find build stage")
	}
	if len(filtered) != 1 || stageName(filtered[0]) != "build" {
		t.Errorf("unexpected filtered result: %v", filtered)
	}

	missing := filterStage(stages, "nonexistent")
	if missing != nil {
		t.Error("expected nil for nonexistent stage")
	}
}

func TestEvaluateCondition(t *testing.T) {
	env := map[string]string{
		"DEPLOY_ENV": "production",
		"VERSION":    "2.0",
	}
	outputs := map[string]map[string]string{
		"build": {"STATUS": "success", "ARTIFACT": "myapp"},
	}

	tests := []struct {
		expr   string
		expect bool
	}{
		{"", true},
		{`${{ env.DEPLOY_ENV == "production" }}`, true},
		{`${{ env.DEPLOY_ENV == "staging" }}`, false},
		{`${{ env.DEPLOY_ENV != "staging" }}`, true},
		{`${{ env.DEPLOY_ENV != "production" }}`, false},
		{`${{ env.DEPLOY_ENV == "production" && env.VERSION == "2.0" }}`, true},
		{`${{ env.DEPLOY_ENV == "production" && env.VERSION == "1.0" }}`, false},
		{`${{ env.DEPLOY_ENV == "staging" || env.VERSION == "2.0" }}`, true},
		{`${{ env.DEPLOY_ENV == "staging" || env.VERSION == "1.0" }}`, false},
		{`${{ steps.build.outputs.STATUS == "success" }}`, true},
		{`${{ steps.build.outputs.STATUS != "success" }}`, false},
		{`${{ steps.build.outputs.ARTIFACT == "myapp" }}`, true},
		{`${{ steps.nonexistent.outputs.FOO == "" }}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			got := evaluateCondition(tc.expr, env, outputs)
			if got != tc.expect {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tc.expr, got, tc.expect)
			}
		})
	}
}

func TestExpandExpressions(t *testing.T) {
	outputs := map[string]map[string]string{
		"build": {"VERSION": "1.2.3", "SHA": "abc123"},
	}
	env := map[string]string{
		"MY_VERSION": "${{ steps.build.outputs.VERSION }}",
		"MY_SHA":     "${{ steps.build.outputs.SHA }}",
		"PLAIN":      "no-expansion-needed",
	}

	result := expandExpressions(env, outputs)
	if result["MY_VERSION"] != "1.2.3" {
		t.Errorf("expected MY_VERSION=1.2.3, got %q", result["MY_VERSION"])
	}
	if result["MY_SHA"] != "abc123" {
		t.Errorf("expected MY_SHA=abc123, got %q", result["MY_SHA"])
	}
	if result["PLAIN"] != "no-expansion-needed" {
		t.Errorf("expected PLAIN unchanged, got %q", result["PLAIN"])
	}
}

func TestTemplateResolution(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/template_step.yaml")
	if err != nil {
		t.Fatal(err)
	}

	err = resolveTemplates(schema, "testdata")
	if err != nil {
		t.Fatalf("template resolution failed: %v", err)
	}

	stages, err := resolveStages(schema)
	if err != nil {
		t.Fatal(err)
	}

	// After resolution, template steps should have Run populated
	steps := stages[0].Steps
	if len(steps) < 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// Second step (go-build with overrides) should have context inputs
	if steps[1].Context == nil {
		t.Fatal("expected context on resolved template step")
	}
	if steps[1].Context.Inputs["go_version"] != "1.23" {
		t.Errorf("expected go_version=1.23 in inputs, got %v", steps[1].Context.Inputs["go_version"])
	}

	// Third step (go-build-defaults) should use template defaults
	if steps[2].Context == nil {
		t.Fatal("expected context on resolved template step with defaults")
	}
	if steps[2].Context.Inputs["go_version"] != "1.22" {
		t.Errorf("expected default go_version=1.22, got %v", steps[2].Context.Inputs["go_version"])
	}

	// Both resolved steps should have a Run field
	if steps[1].Run == nil {
		t.Error("expected Run to be populated after template resolution")
	}
	if steps[2].Run == nil {
		t.Error("expected Run to be populated after template resolution (defaults)")
	}
}

func TestStageTemplateResolution(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/template_stage.yaml")
	if err != nil {
		t.Fatal(err)
	}

	err = resolveTemplates(schema, "testdata")
	if err != nil {
		t.Fatalf("stage template resolution failed: %v", err)
	}

	stages, err := resolveStages(schema)
	if err != nil {
		t.Fatal(err)
	}

	if len(stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(stages))
	}

	// Second stage should be resolved from deploy template
	if stages[1].Context == nil {
		t.Fatal("expected context on resolved stage template")
	}
	if stages[1].Context.Inputs["environment"] != "staging" {
		t.Errorf("expected environment=staging, got %v", stages[1].Context.Inputs["environment"])
	}
	if stages[1].Context.Inputs["version"] != "2.1.0" {
		t.Errorf("expected version=2.1.0, got %v", stages[1].Context.Inputs["version"])
	}

	// Should have steps from the template
	if len(stages[1].Steps) == 0 {
		t.Error("expected steps from template to be populated")
	}
}

func TestForLoopParsing(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/loop_for.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)
	step := stages[0].Steps[0]

	if step.Strategy == nil || step.Strategy.For == nil {
		t.Fatal("expected for strategy")
	}
	if step.Strategy.For.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", step.Strategy.For.Iterations)
	}
}

func TestWhileLoopParsing(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/loop_while.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)
	step := stages[0].Steps[0]

	if step.Strategy == nil || step.Strategy.While == nil {
		t.Fatal("expected while strategy")
	}
	if step.Strategy.While.Iterations != 5 {
		t.Errorf("expected max 5 iterations, got %d", step.Strategy.While.Iterations)
	}
	if step.Strategy.While.Condition == "" {
		t.Error("expected non-empty condition")
	}
}

func TestFailureStrategyParsing(t *testing.T) {
	schema, err := specyaml.ParseFile("testdata/failure_strategy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stages, _ := resolveStages(schema)

	// First stage: retry
	retryStep := stages[0].Steps[0]
	if retryStep.OnFailure == nil || retryStep.OnFailure.Action == nil {
		t.Fatal("expected on-failure with action")
	}
	if retryStep.OnFailure.Action.Retry == nil {
		t.Fatal("expected retry action")
	}
	if retryStep.OnFailure.Action.Retry.Attempts != 3 {
		t.Errorf("expected 3 retry attempts, got %d", retryStep.OnFailure.Action.Retry.Attempts)
	}

	// Second stage: ignore
	ignoreStep := stages[1].Steps[0]
	if ignoreStep.OnFailure == nil || ignoreStep.OnFailure.Action == nil {
		t.Fatal("expected on-failure with ignore action")
	}
	if !ignoreStep.OnFailure.Action.Ignore {
		t.Error("expected ignore=true")
	}
}
