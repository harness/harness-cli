package act

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	specyaml "github.com/bradrydzewski/spec/yaml"
	"github.com/bradrydzewski/spec/yaml/utils/resolver"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type actOptions struct {
	file         string
	dockerHost   string
	dryRun       bool
	stageName    string
	envVars      []string
	network      string
	noPull       bool
	defaultImage string
}

func runAct(cmd *cobra.Command, args []string) error {
	opts := actOptions{file: args[0]}
	opts.dockerHost, _ = cmd.Flags().GetString("docker-host")
	opts.dryRun, _ = cmd.Flags().GetBool("dry-run")
	opts.stageName, _ = cmd.Flags().GetString("stage")
	opts.envVars, _ = cmd.Flags().GetStringSlice("env")
	opts.network, _ = cmd.Flags().GetString("network")
	opts.noPull, _ = cmd.Flags().GetBool("no-pull")
	opts.defaultImage, _ = cmd.Flags().GetString("default-image")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	pterm.Info.Printfln("Parsing pipeline: %s", opts.file)

	schema, err := specyaml.ParseFile(opts.file)
	if err != nil {
		return fmt.Errorf("failed to parse pipeline YAML: %w", err)
	}

	baseDir := filepath.Dir(opts.file)
	if err := resolveTemplates(schema, baseDir); err != nil {
		return fmt.Errorf("template resolution failed: %w", err)
	}

	stages, err := resolveStages(schema)
	if err != nil {
		return err
	}

	if opts.stageName != "" {
		filtered := filterStage(stages, opts.stageName)
		if filtered == nil {
			available := make([]string, 0, len(stages))
			for _, s := range stages {
				available = append(available, stageName(s))
			}
			return fmt.Errorf("stage %q not found; available stages: %s", opts.stageName, strings.Join(available, ", "))
		}
		stages = filtered
	}

	printPipelineSummary(schema, stages)

	if opts.dryRun {
		pterm.Success.Println("Dry run complete — pipeline is valid")
		return nil
	}

	exec, err := newExecutor(ctx, opts)
	if err != nil {
		return err
	}
	defer exec.cleanup()

	return exec.run(ctx, schema, stages)
}

func resolveStages(schema *specyaml.Schema) ([]*specyaml.Stage, error) {
	if schema.Pipeline != nil && len(schema.Pipeline.Stages) > 0 {
		return schema.Pipeline.Stages, nil
	}
	if schema.Pipeline != nil && len(schema.Pipeline.Jobs) > 0 {
		stages := make([]*specyaml.Stage, 0, len(schema.Pipeline.Jobs))
		for name, stage := range schema.Pipeline.Jobs {
			if stage.Name == "" {
				stage.Name = name
			}
			stages = append(stages, stage)
		}
		return stages, nil
	}
	if len(schema.Jobs) > 0 {
		stages := make([]*specyaml.Stage, 0, len(schema.Jobs))
		for name, stage := range schema.Jobs {
			if stage.Name == "" {
				stage.Name = name
			}
			stages = append(stages, stage)
		}
		return stages, nil
	}
	return nil, fmt.Errorf("pipeline has no stages or jobs defined")
}

func filterStage(stages []*specyaml.Stage, name string) []*specyaml.Stage {
	for _, s := range stages {
		if stageName(s) == name {
			return []*specyaml.Stage{s}
		}
	}
	return nil
}

func stageName(s *specyaml.Stage) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Id != "" {
		return s.Id
	}
	return "(unnamed)"
}

func printPipelineSummary(schema *specyaml.Schema, stages []*specyaml.Stage) {
	name := ""
	if schema.Pipeline != nil {
		name = schema.Pipeline.Name
	}
	if name == "" {
		name = schema.Name
	}
	if name == "" {
		name = "(unnamed pipeline)"
	}

	pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack)).
		Println(name)

	tableData := pterm.TableData{{"Stage", "Steps", "Image", "Parallel"}}
	for _, stage := range stages {
		stepCount := countSteps(stage)
		image := inferStageImage(stage)
		parallel := "no"
		if stage.Strategy != nil && stage.Strategy.Matrix != nil {
			axes := len(stage.Strategy.Matrix.Axis)
			parallel = fmt.Sprintf("matrix (%d axes)", axes)
		}
		if stage.Parallel != nil {
			parallel = fmt.Sprintf("group (%d)", len(stage.Parallel.Stages))
		}
		tableData = append(tableData, []string{stageName(stage), fmt.Sprintf("%d", stepCount), image, parallel})
	}
	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	fmt.Println()
}

func countSteps(stage *specyaml.Stage) int {
	count := len(stage.Steps)
	if stage.Group != nil {
		for _, s := range stage.Group.Stages {
			count += countSteps(s)
		}
	}
	return count
}

func inferStageImage(stage *specyaml.Stage) string {
	for _, step := range stage.Steps {
		if step.Run != nil && step.Run.Container != nil && step.Run.Container.Image != "" {
			return step.Run.Container.Image
		}
	}
	return "(default)"
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func resolveTemplates(schema *specyaml.Schema, baseDir string) error {
	return resolver.Resolve(schema, func(name string) (*specyaml.Template, error) {
		templatePath := filepath.Join(baseDir, name)
		tmplSchema, err := specyaml.ParseFile(templatePath)
		if err != nil {
			return nil, fmt.Errorf("cannot load template %q (resolved to %s): %w", name, templatePath, err)
		}
		if tmplSchema.Template == nil {
			return nil, fmt.Errorf("file %q does not contain a template definition", name)
		}
		return tmplSchema.Template, nil
	})
}
