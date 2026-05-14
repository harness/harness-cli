package act

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	specyaml "github.com/bradrydzewski/spec/yaml"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pterm/pterm"
)

type executor struct {
	client       *client.Client
	opts         actOptions
	containers   []string
	extraEnv     map[string]string
	stepOutputs  map[string]map[string]string
	mu           sync.Mutex
	networkID    string
}

func newExecutor(ctx context.Context, opts actOptions) (*executor, error) {
	clientOpts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if opts.dockerHost != "" {
		clientOpts = append(clientOpts, client.WithHost(opts.dockerHost))
	} else if host := resolveDockerContext(); host != "" {
		clientOpts = append(clientOpts, client.WithHost(host))
	}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	spinner, _ := pterm.DefaultSpinner.Start("Connecting to Docker daemon...")
	ping, err := cli.Ping(ctx)
	if err != nil {
		spinner.Fail("Docker daemon is not reachable")
		pterm.Error.Println("Cannot connect to Docker daemon.")
		pterm.Info.Println("Ensure Docker is running and accessible.")
		if opts.dockerHost != "" {
			pterm.Info.Printfln("Configured host: %s", opts.dockerHost)
		} else {
			pterm.Info.Println("Using default Docker socket (set --docker-host to override)")
		}
		return nil, fmt.Errorf("docker daemon unreachable: %w", err)
	}
	spinner.Success(fmt.Sprintf("Connected to Docker (API %s, %s)", ping.APIVersion, ping.OSType))

	extraEnv := make(map[string]string)
	for _, kv := range opts.envVars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			extraEnv[parts[0]] = parts[1]
		}
	}

	exec := &executor{
		client:      cli,
		opts:        opts,
		extraEnv:    extraEnv,
		stepOutputs: make(map[string]map[string]string),
	}

	if opts.network != "" {
		exec.networkID = opts.network
	}

	return exec, nil
}

func (e *executor) cleanup() {
	ctx := context.Background()
	for _, id := range e.containers {
		e.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	}
	e.client.Close()
}

func (e *executor) run(ctx context.Context, schema *specyaml.Schema, stages []*specyaml.Stage) error {
	pipelineStart := time.Now()
	var pipelineEnv map[string]string
	if schema.Pipeline != nil {
		pipelineEnv = schema.Pipeline.Env
	}
	if schema.Env != nil {
		if pipelineEnv == nil {
			pipelineEnv = schema.Env
		} else {
			for k, v := range schema.Env {
				if _, exists := pipelineEnv[k]; !exists {
					pipelineEnv[k] = v
				}
			}
		}
	}

	ordered := topologicalSort(stages)

	for _, batch := range ordered {
		if len(batch) == 1 {
			if err := e.runStage(ctx, batch[0], pipelineEnv); err != nil {
				return err
			}
		} else {
			if err := e.runStagesParallel(ctx, batch, pipelineEnv); err != nil {
				return err
			}
		}
	}

	pterm.Success.Printfln("Pipeline complete in %s", formatDuration(time.Since(pipelineStart)))
	return nil
}

func (e *executor) runStagesParallel(ctx context.Context, stages []*specyaml.Stage, pipelineEnv map[string]string) error {
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = stageName(s)
	}
	pterm.Info.Printfln("Running stages in parallel: %s", strings.Join(names, ", "))

	var wg sync.WaitGroup
	errs := make([]error, len(stages))

	for i, stage := range stages {
		wg.Add(1)
		go func(idx int, s *specyaml.Stage) {
			defer wg.Done()
			errs[idx] = e.runStage(ctx, s, pipelineEnv)
		}(i, stage)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("stage %q failed: %w", stageName(stages[i]), err)
		}
	}
	return nil
}

func (e *executor) runStage(ctx context.Context, stage *specyaml.Stage, pipelineEnv map[string]string) error {
	name := stageName(stage)
	stageStart := time.Now()

	// Evaluate stage-level condition
	if stage.If != "" {
		if !evaluateCondition(stage.If, pipelineEnv, e.stepOutputs) {
			pterm.Info.Printfln("Stage %q skipped (condition: %s)", name, stage.If)
			return nil
		}
	}

	pterm.DefaultSection.Printfln("Stage: %s", name)

	if stage.Parallel != nil {
		return e.runStagesParallel(ctx, stage.Parallel.Stages, pipelineEnv)
	}

	if stage.Group != nil {
		for _, s := range stage.Group.Stages {
			if err := e.runStage(ctx, s, pipelineEnv); err != nil {
				return err
			}
		}
		return nil
	}

	if stage.Strategy != nil && stage.Strategy.Matrix != nil {
		return e.runMatrixStage(ctx, stage, pipelineEnv)
	}

	stageEnv := mergeEnv(pipelineEnv, stage.Env)

	// Propagate stage-level template inputs as env vars
	if stage.Context != nil && stage.Context.Inputs != nil {
		for k, v := range stage.Context.Inputs {
			stageEnv[k] = fmt.Sprintf("%v", v)
		}
	}

	for i, step := range stage.Steps {
		if step.Parallel != nil {
			if err := e.runStepsParallel(ctx, step.Parallel.Steps, stageEnv); err != nil {
				return err
			}
			continue
		}
		if step.Group != nil {
			for _, gs := range step.Group.Steps {
				if err := e.runStep(ctx, gs, stageEnv, i); err != nil {
					return err
				}
			}
			continue
		}
		if err := e.runStep(ctx, step, stageEnv, i); err != nil {
			return err
		}
	}

	pterm.Success.Printfln("Stage %q complete in %s", name, formatDuration(time.Since(stageStart)))
	return nil
}

func (e *executor) runMatrixStage(ctx context.Context, stage *specyaml.Stage, pipelineEnv map[string]string) error {
	matrix := stage.Strategy.Matrix
	combos := expandMatrix(matrix)

	pterm.Info.Printfln("Matrix expansion: %d combinations", len(combos))
	for i, combo := range combos {
		parts := make([]string, 0, len(combo))
		for k, v := range combo {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		pterm.Info.Printfln("  [%d] %s", i+1, strings.Join(parts, ", "))
	}

	maxParallel := int(stage.Strategy.MaxParallel)
	if maxParallel <= 0 {
		maxParallel = len(combos)
	}

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	errs := make([]error, len(combos))

	for i, combo := range combos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, vars map[string]string) {
			defer wg.Done()
			defer func() { <-sem }()
			stageEnv := mergeEnv(pipelineEnv, stage.Env)
			stageEnv = mergeEnv(stageEnv, vars)
			for j, step := range stage.Steps {
				if err := e.runStep(ctx, step, stageEnv, j); err != nil {
					if stage.Strategy.FailFast {
						errs[idx] = err
						return
					}
					pterm.Warning.Printfln("Matrix combo %d step failed: %v", idx+1, err)
				}
			}
		}(i, combo)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("matrix combination %d failed: %w", i+1, err)
		}
	}
	return nil
}

func (e *executor) runStepsParallel(ctx context.Context, steps []*specyaml.Step, env map[string]string) error {
	pterm.Info.Printfln("Running %d steps in parallel", len(steps))
	var wg sync.WaitGroup
	errs := make([]error, len(steps))
	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s *specyaml.Step) {
			defer wg.Done()
			errs[idx] = e.runStep(ctx, s, env, idx)
		}(i, step)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("parallel step %d failed: %w", i+1, err)
		}
	}
	return nil
}

func (e *executor) runStep(ctx context.Context, step *specyaml.Step, env map[string]string, idx int) error {
	name := stepName(step, idx)

	// Evaluate step-level condition
	if step.If != "" {
		if !evaluateCondition(step.If, env, e.stepOutputs) {
			pterm.Info.Printfln("  [%s] skipped (condition: %s)", name, step.If)
			return nil
		}
	}

	// Handle for-loop strategy
	if step.Strategy != nil && step.Strategy.For != nil {
		return e.runStepForLoop(ctx, step, env, idx)
	}

	// Handle while-loop strategy
	if step.Strategy != nil && step.Strategy.While != nil {
		return e.runStepWhileLoop(ctx, step, env, idx)
	}

	// Handle failure strategy wrapping
	err := e.runStepOnce(ctx, step, env, idx, 0)
	if err != nil {
		return e.handleFailureStrategy(ctx, step, env, idx, err)
	}
	return nil
}

func (e *executor) runStepForLoop(ctx context.Context, step *specyaml.Step, env map[string]string, idx int) error {
	iterations := int(step.Strategy.For.Iterations)
	name := stepName(step, idx)
	pterm.Info.Printfln("  [%s] for-loop: %d iterations", name, iterations)

	for i := 0; i < iterations; i++ {
		iterEnv := mergeEnv(env, map[string]string{
			"HARNESS_ITERATION": fmt.Sprintf("%d", i),
		})
		pterm.Info.Printfln("  [%s] iteration %d/%d", name, i+1, iterations)
		if err := e.runStepOnce(ctx, step, iterEnv, idx, 0); err != nil {
			return err
		}
	}
	return nil
}

func (e *executor) runStepWhileLoop(ctx context.Context, step *specyaml.Step, env map[string]string, idx int) error {
	maxIter := int(step.Strategy.While.Iterations)
	condition := step.Strategy.While.Condition
	name := stepName(step, idx)
	pterm.Info.Printfln("  [%s] while-loop: max %d iterations, condition: %s", name, maxIter, condition)

	for i := 0; i < maxIter; i++ {
		// Check condition before each iteration (except first)
		if i > 0 && condition != "" {
			if !evaluateCondition(condition, env, e.stepOutputs) {
				pterm.Info.Printfln("  [%s] while condition no longer true, stopping after %d iterations", name, i)
				break
			}
		}

		iterEnv := mergeEnv(env, map[string]string{
			"HARNESS_ITERATION": fmt.Sprintf("%d", i),
		})
		pterm.Info.Printfln("  [%s] iteration %d/%d", name, i+1, maxIter)
		if err := e.runStepOnce(ctx, step, iterEnv, idx, 0); err != nil {
			return err
		}
	}
	return nil
}

func (e *executor) handleFailureStrategy(ctx context.Context, step *specyaml.Step, env map[string]string, idx int, originalErr error) error {
	if step.OnFailure == nil || step.OnFailure.Action == nil {
		return originalErr
	}

	action := step.OnFailure.Action
	name := stepName(step, idx)

	// Ignore: swallow the error and continue
	if action.Ignore {
		pterm.Warning.Printfln("  [%s] failed but ignored (on-failure: ignore)", name)
		return nil
	}

	// Retry: re-run the step up to N times
	if action.Retry != nil {
		attempts := int(action.Retry.Attempts)
		pterm.Warning.Printfln("  [%s] failed, retrying (up to %d attempts)", name, attempts)
		for attempt := 1; attempt <= attempts; attempt++ {
			pterm.Info.Printfln("  [%s] retry attempt %d/%d", name, attempt, attempts)
			retryEnv := mergeEnv(env, map[string]string{
				"HARNESS_RETRY": fmt.Sprintf("%d", attempt),
			})
			err := e.runStepOnce(ctx, step, retryEnv, idx, attempt)
			if err == nil {
				return nil
			}
			if attempt < attempts {
				pterm.Warning.Printfln("  [%s] attempt %d failed: %v", name, attempt, err)
			} else {
				pterm.Error.Printfln("  [%s] all %d retry attempts exhausted", name, attempts)
				return err
			}
		}
	}

	// Success: mark as success even though it failed
	if action.Success {
		pterm.Warning.Printfln("  [%s] failed but marked as success (on-failure: success)", name)
		return nil
	}

	// Abort (default): propagate the error
	return originalErr
}

func (e *executor) runStepOnce(ctx context.Context, step *specyaml.Step, env map[string]string, idx int, retry int) error {
	name := stepName(step, idx)

	if step.Run == nil && step.Background == nil {
		pterm.Warning.Printfln("  [%s] Skipping non-run step (type not supported locally)", name)
		return nil
	}

	run := step.Run
	if run == nil {
		run = step.Background
	}

	img := e.opts.defaultImage
	if run.Container != nil && run.Container.Image != "" {
		img = run.Container.Image
	}

	script := strings.Join(run.Script, "\n")
	if script == "" {
		pterm.Warning.Printfln("  [%s] Empty script, skipping", name)
		return nil
	}

	stepEnv := mergeEnv(env, step.Env)
	stepEnv = mergeEnv(stepEnv, run.Env)
	stepEnv = mergeEnv(stepEnv, e.extraEnv)

	// Add template inputs as env vars if present
	if step.Context != nil && step.Context.Inputs != nil {
		for k, v := range step.Context.Inputs {
			stepEnv[k] = fmt.Sprintf("%v", v)
		}
	}

	// Expand ${{ ... }} expressions in env values
	stepEnv = expandExpressions(stepEnv, e.stepOutputs)

	shell := run.Shell
	if shell == "" {
		shell = "sh"
	}

	stepStart := time.Now()
	prefix := pterm.LightBlue(fmt.Sprintf("  [%s]", name))
	if retry == 0 {
		fmt.Printf("%s image=%s shell=%s\n", prefix, img, shell)
	}

	if !e.opts.noPull && retry == 0 {
		if err := e.pullImage(ctx, img); err != nil {
			return fmt.Errorf("step %q: pull image %q: %w", name, img, err)
		}
	}

	// Wrap script to capture output variables via HARNESS_OUTPUT
	outputFile := "/tmp/.harness_output"
	wrappedScript := fmt.Sprintf("export HARNESS_OUTPUT=%s\ntouch %s\n%s", outputFile, outputFile, script)

	containerEnv := make([]string, 0, len(stepEnv))
	for k, v := range stepEnv {
		containerEnv = append(containerEnv, k+"="+v)
	}

	var entrypoint []string
	var cmd []string
	switch shell {
	case "bash":
		entrypoint = []string{"/bin/bash"}
		cmd = []string{"-e", "-c", wrappedScript}
	case "python":
		entrypoint = []string{"python3"}
		cmd = []string{"-c", wrappedScript}
	case "powershell", "pwsh":
		entrypoint = []string{"pwsh"}
		cmd = []string{"-Command", wrappedScript}
	default:
		entrypoint = []string{"/bin/sh"}
		cmd = []string{"-e", "-c", wrappedScript}
	}

	cfg := &container.Config{
		Image:      img,
		Env:        containerEnv,
		Entrypoint: entrypoint,
		Cmd:        cmd,
		WorkingDir: "/workspace",
	}

	hostCfg := &container.HostConfig{}
	if run.Container != nil && run.Container.Privileged {
		hostCfg.Privileged = true
	}

	var networkCfg *network.NetworkingConfig
	if e.networkID != "" {
		networkCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				e.networkID: {},
			},
		}
	}

	resp, err := e.client.ContainerCreate(ctx, cfg, hostCfg, networkCfg, nil, "")
	if err != nil {
		return fmt.Errorf("step %q: create container: %w", name, err)
	}

	e.mu.Lock()
	e.containers = append(e.containers, resp.ID)
	e.mu.Unlock()

	if err := e.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("step %q: start container: %w", name, err)
	}

	logReader, err := e.client.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return fmt.Errorf("step %q: attach logs: %w", name, err)
	}
	defer logReader.Close()
	streamLogs(logReader, prefix)

	waitCh, errCh := e.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		elapsed := time.Since(stepStart)
		if result.StatusCode != 0 {
			pterm.Error.Printfln("  [%s] exited %d (%s)", name, result.StatusCode, formatDuration(elapsed))
			return fmt.Errorf("step %q exited with code %d", name, result.StatusCode)
		}
		pterm.Success.Printfln("  [%s] done (%s)", name, formatDuration(elapsed))
	case err := <-errCh:
		return fmt.Errorf("step %q: wait: %w", name, err)
	case <-ctx.Done():
		e.client.ContainerKill(context.Background(), resp.ID, "KILL")
		return ctx.Err()
	}

	// Capture output variables from the container
	e.captureOutputs(ctx, resp.ID, step, idx, outputFile)

	return nil
}

func (e *executor) captureOutputs(ctx context.Context, containerID string, step *specyaml.Step, idx int, outputFile string) {
	stepID := step.Id
	if stepID == "" {
		stepID = step.Name
	}
	if stepID == "" {
		return
	}

	reader, _, err := e.client.CopyFromContainer(ctx, containerID, outputFile)
	if err != nil {
		return
	}
	defer reader.Close()

	// CopyFromContainer returns a tar archive
	data, err := io.ReadAll(reader)
	if err != nil {
		return
	}

	// Extract file content from tar (skip 512-byte header)
	content := extractTarContent(data)
	if content == "" {
		return
	}

	outputs := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			outputs[parts[0]] = parts[1]
		}
	}

	if len(outputs) > 0 {
		e.mu.Lock()
		e.stepOutputs[stepID] = outputs
		e.mu.Unlock()
		pterm.Info.Printfln("  [%s] captured %d output variable(s)", stepID, len(outputs))
	}
}

func extractTarContent(data []byte) string {
	// tar format: 512 byte header, then file content
	if len(data) <= 512 {
		return ""
	}
	// Find the file size from the tar header (bytes 124-135, octal)
	sizeField := strings.TrimRight(string(data[124:135]), "\x00 ")
	var size int64
	fmt.Sscanf(sizeField, "%o", &size)
	if size <= 0 || int64(len(data)) < 512+size {
		return ""
	}
	return string(data[512 : 512+size])
}

func (e *executor) pullImage(ctx context.Context, img string) error {
	reader, err := e.client.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
	return nil
}

func stepName(step *specyaml.Step, idx int) string {
	if step.Name != "" {
		return step.Name
	}
	if step.Id != "" {
		return step.Id
	}
	return fmt.Sprintf("step-%d", idx+1)
}

func streamLogs(r io.Reader, prefix string) {
	buf := make([]byte, 4096)
	var lineBuf bytes.Buffer
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			for len(chunk) > 0 {
				idx := bytes.IndexByte(chunk, '\n')
				if idx == -1 {
					lineBuf.Write(chunk)
					break
				}
				lineBuf.Write(chunk[:idx])
				line := lineBuf.String()
				// Docker multiplexed stream has 8-byte header per frame
				if len(line) >= 8 {
					line = stripDockerHeader(line)
				}
				fmt.Printf("%s %s\n", prefix, line)
				lineBuf.Reset()
				chunk = chunk[idx+1:]
			}
		}
		if err != nil {
			if lineBuf.Len() > 0 {
				line := lineBuf.String()
				if len(line) >= 8 {
					line = stripDockerHeader(line)
				}
				fmt.Printf("%s %s\n", prefix, line)
			}
			break
		}
	}
}

func stripDockerHeader(line string) string {
	if len(line) < 8 {
		return line
	}
	// Docker multiplexed log format: [stream_type][0][0][0][size_bytes x4][payload]
	if line[0] <= 2 && line[1] == 0 && line[2] == 0 && line[3] == 0 {
		return line[8:]
	}
	return line
}

func mergeEnv(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func expandMatrix(matrix *specyaml.Matrix) []map[string]string {
	keys := make([]string, 0, len(matrix.Axis))
	for k := range matrix.Axis {
		keys = append(keys, k)
	}

	var results []map[string]string
	var expand func(idx int, current map[string]string)
	expand = func(idx int, current map[string]string) {
		if idx == len(keys) {
			combo := make(map[string]string, len(current))
			for k, v := range current {
				combo[k] = v
			}
			results = append(results, combo)
			return
		}
		key := keys[idx]
		for _, val := range matrix.Axis[key] {
			current[key] = val
			expand(idx+1, current)
		}
	}
	expand(0, make(map[string]string))

	if len(matrix.Include) > 0 {
		results = append(results, matrix.Include...)
	}

	if len(matrix.Exclude) > 0 {
		filtered := results[:0]
		for _, combo := range results {
			excluded := false
			for _, excl := range matrix.Exclude {
				match := true
				for k, v := range excl {
					if combo[k] != v {
						match = false
						break
					}
				}
				if match {
					excluded = true
					break
				}
			}
			if !excluded {
				filtered = append(filtered, combo)
			}
		}
		results = filtered
	}

	return results
}

// resolveDockerContext reads ~/.docker/config.json to find the active context,
// then reads the context metadata to get the docker host endpoint.
// This mirrors what `docker ps` does — the Go SDK's client.FromEnv only checks
// DOCKER_HOST and doesn't know about Docker CLI contexts.
func resolveDockerContext() string {
	if os.Getenv("DOCKER_HOST") != "" {
		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPath := filepath.Join(home, ".docker", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.CurrentContext == "" {
		return ""
	}

	// Docker stores context metadata under a SHA-256 hash of the context name
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.CurrentContext)))
	metaPath := filepath.Join(home, ".docker", "contexts", "meta", hash, "meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}

	var meta struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return ""
	}

	if ep, ok := meta.Endpoints["docker"]; ok && ep.Host != "" {
		return ep.Host
	}
	return ""
}

func topologicalSort(stages []*specyaml.Stage) [][]*specyaml.Stage {
	nameMap := make(map[string]*specyaml.Stage)
	for _, s := range stages {
		nameMap[stageName(s)] = s
	}

	done := make(map[string]bool)
	var batches [][]*specyaml.Stage

	for len(done) < len(stages) {
		var batch []*specyaml.Stage
		for _, s := range stages {
			n := stageName(s)
			if done[n] {
				continue
			}
			ready := true
			for _, dep := range s.Needs {
				if !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				batch = append(batch, s)
			}
		}
		if len(batch) == 0 {
			// circular dependency - just run remaining sequentially
			for _, s := range stages {
				if !done[stageName(s)] {
					batch = append(batch, s)
					break
				}
			}
		}
		for _, s := range batch {
			done[stageName(s)] = true
		}
		batches = append(batches, batch)
	}

	return batches
}

