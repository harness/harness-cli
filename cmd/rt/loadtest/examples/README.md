# Harness RT load test example configurations

Working `--config` files for every tool and authoring mode. Each one is a
complete definition: copy it, change the identifiers, and run it.

Every file here is checked by `go test ./cmd/rt/loadtest/api/ -run
TestCommittedExamplesDecode`, which decodes it with unknown fields rejected —
the same strictness `--config` applies. An example cannot drift from the
schema without turning the build red.

## The files

| File | Tool | Mode | Shows |
|------|------|------|-------|
| [`k6-script.json`](k6-script.json) | k6 | script | Thresholds, metric tags, k6 options, a runtime input for `targetUsers` |
| [`k6-image.json`](k6-image.json) | k6 | image | Prebuilt image with an entrypoint and run arguments, iteration and RPS caps |
| [`jmeter-script.json`](jmeter-script.json) | JMeter | script | JMeter properties driving `__P()` in the plan, latency and error thresholds |
| [`jmeter-image.json`](jmeter-image.json) | JMeter | image | Distributed run across three injectors, `retain` cleanup policy |
| [`locust-script.json`](locust-script.json) | Locust | script | `spawnRate`, a secret env var sourced from a Harness secret |
| [`locust-image.json`](locust-image.json) | Locust | image | Distributed Locust across four workers |
| [`locust-from-json.json`](locust-from-json.json) | Locust | JSON spec | Declarative endpoints, token extraction, whole definition in one file |
| [`journey.json`](journey.json) | Locust | JSON spec | The bare endpoint spec on its own, for `--spec` |

The scripts the script-mode examples refer to live in
[`scripts/`](scripts): `checkout.js` (k6), `checkout.jmx` (JMeter) and
`locustfile.py` (Locust).

## Running them

Script-mode examples do not inline the script. `script.content` holds a
base64 blob that is unreadable in a diff, so the config leaves it out and
`--script` fills it in:

```bash
hc rt loadtest create --config ./k6-script.json --script ./scripts/checkout.js
hc rt loadtest create --config ./jmeter-script.json --script ./scripts/checkout.jmx
hc rt loadtest create --config ./locust-script.json --script ./scripts/locustfile.py
```

Image-mode examples are self-contained:

```bash
hc rt loadtest create --config ./k6-image.json
```

A JSON spec works either way — the whole definition in one file, or the spec
on its own with the rest supplied as flags:

```bash
hc rt loadtest create-from-json --config ./locust-from-json.json

hc rt loadtest create-from-json --spec ./journey.json \
  --identity api-journey --name "API journey" \
  --infra-id perf-cluster --environment-id staging \
  --target-users 100 --duration 600
```

## Adapting one

Three fields are environment-specific and must be changed before any of these
will run against your account:

- `identity` — unique per project.
- `infraIdentifier` — list available infrastructures in the Harness UI.
- `environmentIdentifier` — the environment that infrastructure belongs to.

Flags override the file, so one config can serve several runs without being
edited:

```bash
hc rt loadtest create --config ./k6-script.json --script ./scripts/checkout.js \
  --identity checkout-k6-eu --target-users 500 --duration 1800
```

To start from a blank template instead of one of these, generate one for any
tool and mode:

```bash
hc rt loadtest create --generate-config-skeleton --tool-type K6 --mode script
hc rt loadtest create-from-json --generate-config-skeleton
```

## Two things worth knowing

**`<+input>` defers a value to run time.** Any leaf can hold it instead of a
literal — `k6-script.json` uses it for `targetUsers` so the same test can be
run at different volumes:

```bash
hc rt loadtest start checkout-k6 --runtime-value k6.tunables.targetUsers=500
```

Note the tool key. When authoring, `--runtime-input tunables.targetUsers` is
relative to the block `--tool-type` already selected. At run time there is no
`--tool-type`, so `--runtime-value` takes the full path from the root of
`toolConfig`: `k6.tunables.targetUsers`.

**The spec sub-schema is snake_case.** Inside `jsonSpec`, fields are
`min_wait`, `max_wait`, `query_params`, `status_code`,
`max_response_time_ms`, `variable_name` and `json_path` — unlike the
camelCase used everywhere else in a definition. This follows the API.
