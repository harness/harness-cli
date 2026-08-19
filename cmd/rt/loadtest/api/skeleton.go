package api

// Skeleton is AWS CLI's --generate-cli-skeleton, but filled with realistic
// values rather than zero ones: the tool vocabulary is hard to fill in blind.
func Skeleton(tool ToolType, mode Mode) map[string]any {
	if mode != ModeImage {
		mode = ModeScript
	}

	definition := map[string]any{
		"identity":              "checkout-load",
		"name":                  "Checkout load",
		"description":           "Sustained load against the checkout path",
		"toolType":              string(tool),
		"infraIdentifier":       "perf-cluster",
		"environmentIdentifier": "staging",
		"targetType":            string(TargetKubernetes),
		"tags":                  []string{"nightly", "checkout"},
		"serviceReferences":     []string{"checkout-service"},
		"cleanupPolicy":         string(CleanupDelete),
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "500m", "memory": "1Gi"},
			"limits":   map[string]any{"cpu": "2", "memory": "2Gi"},
		},
		"variables": []any{
			map[string]any{
				"name":        "baseUrl",
				"value":       "https://staging.example.com",
				"type":        "string",
				"description": "Referenced from toolConfig as <+variable.baseUrl>",
			},
		},
		"toolConfig": map[string]any{
			ToolKey(tool): toolSkeleton(tool, mode),
		},
	}

	return definition
}

func toolSkeleton(tool ToolType, mode Mode) map[string]any {
	block := map[string]any{
		"mode":     string(mode),
		"script":   scriptSkeleton(tool, mode),
		"tunables": tunablesSkeleton(tool),
		"envVars": []any{
			map[string]any{"key": "REGION", "value": "us-east-1"},
			map[string]any{"key": "API_TOKEN", "value": "account.perfApiToken", "secret": true},
		},
	}

	switch tool {
	case ToolJMeter:
		block["properties"] = []any{
			map[string]any{"key": "threads", "value": 200, "sendToEngines": true},
		}
		block["thresholds"] = []any{
			map[string]any{"metric": "error_rate_pct", "operator": "<", "value": 1, "abortOnFail": true},
			map[string]any{"metric": "response_time_ms", "stat": "p95", "operator": "<", "value": 500},
		}
	case ToolK6:
		block["metricTags"] = map[string]any{"service": "checkout", "build": "<+input>"}
		block["options"] = map[string]any{
			"userAgent":             "harness-loadtest/1.0",
			"dnsStrategy":           "round-robin",
			"noConnectionReuse":     false,
			"insecureSkipTLSVerify": false,
			"batchConcurrent":       20,
			"batchPerHost":          6,
			"discardResponseBodies": false,
		}
		block["thresholds"] = []any{
			map[string]any{"metric": "http_req_failed", "operator": "<", "value": 0.01, "abortOnFail": true},
			map[string]any{"metric": "http_req_duration", "stat": "p95", "operator": "<", "value": 500},
		}
	}

	return block
}

func scriptSkeleton(tool ToolType, mode Mode) map[string]any {
	if mode == ModeImage {
		script := map[string]any{
			"source":          string(SourceImage),
			"image":           imageExample(tool),
			"entrypoint":      "",
			"runArgs":         []string{},
			"imagePullSecret": "",
		}
		return script
	}

	script := map[string]any{
		"source": string(SourceInline),
		// The base64-encoded script body, which "--script ./file" fills in.
		"content": "",
	}
	if tool == ToolJMeter {
		// testPlan names the main plan inside a .zip workspace, and is ignored
		// for a bare .jmx.
		script["testPlan"] = "checkout.jmx"
	}
	return script
}

func imageExample(tool ToolType) string {
	switch tool {
	case ToolJMeter:
		return "ghcr.io/example/jmeter-suite:1.0.0"
	case ToolLocust:
		return "ghcr.io/example/locust-suite:1.0.0"
	case ToolK6:
		return "ghcr.io/example/k6-suite:1.0.0"
	}
	return ""
}

// tunablesSkeleton emits only the tunables the engine actually reads, so the
// template does not suggest a setting that would be ignored.
func tunablesSkeleton(tool ToolType) map[string]any {
	tunables := map[string]any{
		"targetUrl": "<+variable.baseUrl>",
		// Any leaf may be "<+input>", deferring it to "start --runtime-value".
		"targetUsers":     200,
		"rampUpTimeSec":   60,
		"durationSeconds": 900,
		"workerCount":     1,
		"maxDurationSec":  3600,
	}

	switch tool {
	case ToolLocust:
		tunables["spawnRate"] = 5
	case ToolK6:
		tunables["hostUrl"] = "<+variable.baseUrl>"
		tunables["iterations"] = 0
		tunables["rpsLimit"] = 0
	}

	return tunables
}

// JSONSpecSkeleton builds a declarative endpoint spec, the third authoring
// mode alongside script and image.
func JSONSpecSkeleton() map[string]any {
	minWait, maxWait := 1, 3
	status, maxMs := 200, 500

	definition := Skeleton(ToolLocust, ModeScript)
	definition["identity"] = "api-journey"
	definition["name"] = "API journey"
	definition["description"] = "Login, then list orders as an authenticated user"
	definition["variables"] = []any{
		map[string]any{
			"name":        "perfPassword",
			"value":       "account.perfPassword",
			"type":        "secret",
			"description": "Referenced from an endpoint body as <+variable.perfPassword>",
		},
	}

	// A JSON spec replaces the script, so the artifact block has no meaning.
	if block, ok := definition["toolConfig"].(map[string]any)[ToolKey(ToolLocust)].(map[string]any); ok {
		delete(block, "script")
		delete(block, "mode")
		if tunables, ok := block["tunables"].(map[string]any); ok {
			// The endpoint spec supplies the host, so targetUrl would be a
			// second, conflicting answer to the same question.
			delete(tunables, "targetUrl")
		}
	}

	// The from-json endpoint takes neither of these, so emitting them would
	// produce a template that --config rejects.
	delete(definition, "cleanupPolicy")
	delete(definition, "resources")

	definition["jsonSpec"] = JSONSpec{
		Config: JSONSpecConfig{
			Host:    "https://api.staging.example.com",
			MinWait: &minWait,
			MaxWait: &maxWait,
		},
		Endpoints: []JSONSpecEndpoint{
			{
				Name:       "login",
				Method:     "POST",
				Path:       "/v1/auth/login",
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       map[string]any{"user": "perf", "password": "<+variable.perfPassword>"},
				Assertions: &JSONSpecAsserts{StatusCode: &status, MaxResponseTimeMs: &maxMs},
				Extract:    &JSONSpecExtract{VariableName: "token", JSONPath: "$.data.accessToken"},
			},
			{
				Name:        "list-orders",
				Method:      "GET",
				Path:        "/v1/orders",
				QueryParams: map[string]string{"limit": "20"},
				Headers:     map[string]string{"Authorization": "Bearer {{token}}"},
				Assertions:  &JSONSpecAsserts{StatusCode: &status},
			},
		},
	}

	return definition
}
