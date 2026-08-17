# Push/pull package URL resolution (`--pkg-url`, `--api-url`)

Implements `docs/hc-open-issues-fix-plan.md` §3 (P2b). Previously the package-upload
client was built **only** from `config.Global.Registry.PkgURL`, which is filled solely
from `auth.json`'s `registry_url`. With explicit `--account/--api-url/--token` and no
`--pkg-url`, pushes streamed bytes to a stale/empty package host and failed with
`404 {"message":"Root not found: <account>"}`.

## Resolution order

`cmdutils.ResolvePkgURL(cmd, pkgURLFlag)` runs in the `PreRunE` of every push/pull
command that supports `--pkg-url` and settles `config.Global.Registry.PkgURL` before
any bytes are streamed:

1. **`--pkg-url` flag** — normalized via `util.GetPkgUrl` (adds `https://` when the
   scheme is missing). `--pkg-url` **remains supported** for explicit-credential
   flows; it is not deprecated.
2. **Saved config** — a package URL already loaded from `auth.json`
   (`hc auth login`) is left untouched.
3. **Derived from `--api-url`** — only when `--api-url` was explicitly set on the
   command line and no package URL is configured yet: the CLI calls
   `GET <api-url>/gateway/har/api/v3/system/info` and reads `data.registryUrl`,
   the same resolution `hc auth login` performs. The resolved (secret-free)
   endpoint is logged at info/debug.

If the endpoint is still empty, the command **fails fast** with
`pkg-url must be set: ...` before uploading anything (previously only the npm and
dart pushes had this check; it is now generalized to every wired command).

## Wired commands

- `artifact push`: generic, maven, go, conda, composer, rpm, cargo, npm, dart,
  python, nuget
- `artifact pull generic`

`registry configure npm` (`cmd/registry/command/configure_npm.go`) and
`registry migrate` share the same `--pkg-url` pattern but were not wired in this
change (separate ownership); they can adopt `cmdutils.ResolvePkgURL` the same way.

## Terraform / debian / conan / swift / puppet pushes

These push commands intentionally do **not** get a `--pkg-url` flag (decided
2026-08-07, plan §3-W4). In particular, **hc is unsupported for Terraform bridge
pushes with explicit credentials** — there is no URL override escape hatch on that
path. Use `hc auth login` (saved config) or `scripts/art-tf-provider-migrate.sh`,
which remains the supported path for Terraform provider mirroring.

## Regression check

`scripts/push-url-ab-test.sh` is a **manually run** A/B/C script against DEV that
verifies: leg A (explicit credentials, no `--pkg-url`) now succeeds via derivation,
leg B (explicit credentials + `--pkg-url`) still works, and leg C (saved config) is
unchanged.
