#!/usr/bin/env bash
# Test suite for the Metronome IDC credential source feature.
#
# Covers:
#   1. Substrate path is unaffected when IDC is not configured.
#   2. configure --use-identitycenter routes all commands through IDC.
#   3. USE_SUBSTRATE=true forces Substrate even when IDC is configured.
#   4. configure --use-substrate reverts to Substrate.
#
# Usage:
#   ./scripts/identitycenter_test.sh [--engineersreadonly] [--run-configure]
#
#   Requires: access-metronome-aws-admin LMS permission
#   --engineersreadonly  Use -r engineersreadonly for all assume commands (for testing
#                        before admin permission sets are provisioned).
#   --run-configure      Write real ~/.aws/config and ~/.kube/config during configure tests.

set -euo pipefail

# ── flags ────────────────────────────────────────────────────────────────────
RUN_CONFIGURE=false
ENGINEERSREADONLY=false
for arg in "$@"; do
  case "$arg" in
    --engineersreadonly) ENGINEERSREADONLY=true ;;
    --run-configure)     RUN_CONFIGURE=true ;;
    *) echo "Unknown flag: $arg"; exit 1 ;;
  esac
done

ROLE_FLAG=""
[[ "$ENGINEERSREADONLY" == "true" ]] && ROLE_FLAG="-r engineersreadonly"

# ── harness ──────────────────────────────────────────────────────────────────
PASS=0; FAIL=0; SKIP=0
FAILURES=()

pass() { echo "  PASS  $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL  $1"; FAIL=$((FAIL + 1)); FAILURES+=("$1"); }
skip() { echo "  SKIP  $1"; SKIP=$((SKIP + 1)); }

assert_ok() {
  local desc="$1"; shift
  local out
  if out=$("$@" 2>&1); then
    pass "$desc"
  else
    fail "$desc — exit $? — $(echo "$out" | head -2)"
  fi
}

assert_fail() {
  local desc="$1"; shift
  if "$@" > /dev/null 2>&1; then
    fail "$desc — expected non-zero but got 0"
  else
    pass "$desc"
  fi
}

assert_output() {
  local desc="$1" pattern="$2"; shift 2
  local out
  out=$("$@" 2>&1)
  local code=$?
  if [[ $code -ne 0 ]]; then
    fail "$desc — exit $code"
  elif echo "$out" | rg -q "$pattern"; then
    pass "$desc"
  else
    fail "$desc — pattern '$pattern' not found in: $(echo "$out" | head -2)"
  fi
}

assert_json() {
  local desc="$1"; shift
  local out
  out=$("$@" 2>&1)
  local code=$?
  if [[ $code -ne 0 ]]; then
    fail "$desc — exit $code"
  elif echo "$out" | python3 -m json.tool > /dev/null 2>&1; then
    pass "$desc"
  else
    fail "$desc — not valid JSON: $(echo "$out" | head -2)"
  fi
}

# Extract a top-level field from a JSON string passed on stdin
json_field() { python3 -c "import sys,json; print(json.load(sys.stdin).get('$1',''))"; }

# Print the AWS account ID that a JSON credentials blob resolves to.
aws_account_for_creds() {
  local json="$1"
  local key secret token
  key=$(echo "$json"    | json_field AccessKeyId)
  secret=$(echo "$json" | json_field SecretAccessKey)
  token=$(echo "$json"  | json_field SessionToken)
  AWS_ACCESS_KEY_ID="$key" AWS_SECRET_ACCESS_KEY="$secret" AWS_SESSION_TOKEN="$token" \
    aws sts get-caller-identity --query Account --output text 2>/dev/null
}

# Remove all IDC credential cache files so tests don't reuse stale credentials.
idc_clear_creds() {
  rm -f ~/.quikstrate/credentials-idc.json
  rm -f ~/.quikstrate/*-idc.json 2>/dev/null || true
}

# Remove every cache file (Substrate and IDC) for the env/domain and special
# account fixtures below, so leftovers from earlier runs — including runs of
# the real quikstrate binary against the same accounts — can't be mistaken
# for files this run wrote.
clear_fixture_caches() {
  local key acct_env acct_domain
  for key in "${!SERVICE_ACCOUNTS[@]}"; do
    acct_env="${key%%-*}"
    acct_domain="${key#*-}"
    rm -f ~/.quikstrate/"${acct_env}-${acct_domain}"-*.json 2>/dev/null || true
  done
  for key in "${!SPECIAL_ACCOUNTS[@]}"; do
    rm -f ~/.quikstrate/special-"${key}"-*.json 2>/dev/null || true
  done
}

section() { echo; echo "── $1 ──────────────────────────────────────────────"; }

# ── account fixtures ──────────────────────────────────────────────────────────
declare -A SERVICE_ACCOUNTS=(
  ["staging-api"]="407752757973"
  ["staging-graphql"]="008444403661"
  ["prod-api"]="477056945755"
  ["prod-graphql"]="051318803586"
  ["prod-internal-services"]="207567762512"
)
declare -A SPECIAL_ACCOUNTS=(
  ["management"]="420073272039"
  ["audit"]="465454680116"
  ["deploy"]="703712742941"
  ["network"]="814412579886"
)

CRED_FIELDS=(Version AccessKeyId SecretAccessKey SessionToken)

# ── prerequisites ─────────────────────────────────────────────────────────────
section "Prerequisites"

BIN="${QUIKSTRATE_BIN:-quikstrate}"
if ! command -v "$BIN" > /dev/null 2>&1; then
  echo "ERROR: '$BIN' not found in PATH."
  echo "       Build: go build -o quikstrate . && export PATH=\$PWD:\$PATH"
  echo "       Or:    export QUIKSTRATE_BIN=/path/to/binary"
  exit 1
fi
echo "  binary: $(command -v "$BIN")"

if ! aws sts get-caller-identity > /dev/null 2>&1; then
  echo "ERROR: No working AWS credentials. Source credentials before running."
  exit 1
fi
echo "  AWS identity: $(aws sts get-caller-identity --query Arn --output text)"

echo "  IDC: login will be triggered automatically on first credential fetch if needed"

clear_fixture_caches
echo "  cleared cached credentials for fixture accounts"

# Save the original config so it can be restored on exit regardless of outcome.
SAVED_CONFIG=""
if [[ -f ~/.quikstrate/config.json ]]; then
  SAVED_CONFIG=$(cat ~/.quikstrate/config.json)
fi

restore_config() {
  if [[ -n "$SAVED_CONFIG" ]]; then
    echo "$SAVED_CONFIG" > ~/.quikstrate/config.json
  else
    rm -f ~/.quikstrate/config.json
  fi
}
trap restore_config EXIT

# ── Section 1: Substrate path (default, no IDC config) ───────────────────────

section "Substrate: default (no IDC config)"

rm -f ~/.quikstrate/config.json
"$BIN" credentials --force --format json > /dev/null 2>&1

assert_ok "Substrate: credentials.json written" \
  test -f ~/.quikstrate/credentials.json

if [[ -f ~/.quikstrate/credentials-idc.json ]]; then
  substrate_mtime=$(stat -f %m ~/.quikstrate/credentials.json  2>/dev/null || stat -c %Y ~/.quikstrate/credentials.json  2>/dev/null)
  idc_mtime=$(stat -f %m ~/.quikstrate/credentials-idc.json 2>/dev/null || stat -c %Y ~/.quikstrate/credentials-idc.json 2>/dev/null)
  if [[ "$idc_mtime" -ge "$substrate_mtime" ]]; then
    fail "Substrate: credentials-idc.json was freshly written (should not be)"
  else
    pass "Substrate: credentials-idc.json not freshly written"
  fi
else
  pass "Substrate: credentials-idc.json does not exist"
fi

# ── Section 2: configure --use-identitycenter ────────────────────────────────

section "IDC: configure --use-identitycenter"

AWS_CONFIG_BEFORE=$(md5sum ~/.aws/config 2>/dev/null || true)
"$BIN" configure --use-identitycenter --dryrun > /dev/null 2>&1
[[ ! -f ~/.quikstrate/config.json ]] \
  && pass "IDC: configure --dryrun did not create config.json" \
  || fail "IDC: configure --dryrun created config.json"
AWS_CONFIG_AFTER=$(md5sum ~/.aws/config 2>/dev/null || true)
[[ "$AWS_CONFIG_BEFORE" == "$AWS_CONFIG_AFTER" ]] \
  && pass "IDC: configure --dryrun did not modify ~/.aws/config" \
  || fail "IDC: configure --dryrun modified ~/.aws/config"

if [[ "$RUN_CONFIGURE" == "true" ]]; then
  "$BIN" configure --use-identitycenter > /dev/null 2>&1
else
  mkdir -p ~/.quikstrate
  echo '{"credential_source":"identitycenter"}' > ~/.quikstrate/config.json
  echo
  echo "  INFO  Pass --run-configure to write ~/.aws/config and test AWS_PROFILE end-to-end."
fi

if [[ -f ~/.quikstrate/config.json ]]; then
  cfg_source=$(python3 -c "import json; print(json.load(open('$HOME/.quikstrate/config.json')).get('credential_source',''))")
  [[ "$cfg_source" == "identitycenter" ]] \
    && pass "IDC: config.json has credential_source=identitycenter" \
    || fail "IDC: config.json has unexpected credential_source=$cfg_source"
else
  fail "IDC: config.json not written by configure --use-identitycenter"
fi

if [[ "$RUN_CONFIGURE" == "true" ]]; then
  python3 -c "
import sys
data = open('$HOME/.aws/config').read()
sys.exit(0 if '[sso-session metronome]' in data else 1)
" 2>/dev/null \
    && pass "IDC: [sso-session metronome] block in ~/.aws/config" \
    || fail "IDC: [sso-session metronome] block missing from ~/.aws/config"

  "$BIN" configure --use-identitycenter > /dev/null 2>&1 || true
  block_count=$(python3 -c "print(open('$HOME/.aws/config').read().count('[sso-session metronome]'))")
  [[ "$block_count" == "1" ]] \
    && pass "IDC: configure --use-identitycenter is idempotent (1 sso-session block)" \
    || fail "IDC: configure --use-identitycenter produced $block_count sso-session blocks"
else
  skip "IDC: sso-session block check (pass --run-configure)"
  skip "IDC: configure idempotency check (pass --run-configure)"
fi

# ── Section 3: IDC credentials ───────────────────────────────────────────────

section "IDC: cache isolation"

SUBSTRATE_MTIME_BEFORE=$(stat -f %m ~/.quikstrate/credentials.json 2>/dev/null \
  || stat -c %Y ~/.quikstrate/credentials.json 2>/dev/null || echo 0)

idc_clear_creds
"$BIN" credentials --force --format json > /dev/null 2>&1

assert_ok "IDC: credentials-idc.json written after --force" \
  test -f ~/.quikstrate/credentials-idc.json

SUBSTRATE_MTIME_AFTER=$(stat -f %m ~/.quikstrate/credentials.json 2>/dev/null \
  || stat -c %Y ~/.quikstrate/credentials.json 2>/dev/null || echo 0)
if [[ "$SUBSTRATE_MTIME_AFTER" == "$SUBSTRATE_MTIME_BEFORE" ]]; then
  pass "IDC: credentials.json not modified by IDC fetch"
else
  fail "IDC: credentials.json was modified during IDC fetch"
fi

section "IDC: base credentials"

idc_clear_creds
IDC_CREDS=$("$BIN" credentials --format json 2>/dev/null)

if echo "$IDC_CREDS" | python3 -m json.tool > /dev/null 2>&1; then
  pass "IDC: credentials --format json is valid JSON"
else
  fail "IDC: credentials --format json is not valid JSON"
fi
for field in "${CRED_FIELDS[@]}"; do
  val=$(echo "$IDC_CREDS" | json_field "$field")
  [[ -n "$val" ]] && pass "IDC: credentials has $field" || fail "IDC: credentials missing $field"
done

IDC_KEY=$(echo "$IDC_CREDS" | json_field AccessKeyId)
IDC_SECRET=$(echo "$IDC_CREDS" | json_field SecretAccessKey)
IDC_TOKEN=$(echo "$IDC_CREDS" | json_field SessionToken)
assert_ok "IDC: credentials work (sts get-caller-identity)" \
  env AWS_ACCESS_KEY_ID="$IDC_KEY" AWS_SECRET_ACCESS_KEY="$IDC_SECRET" AWS_SESSION_TOKEN="$IDC_TOKEN" \
  aws sts get-caller-identity

assert_ok "IDC: credentials --check exits 0 after fresh fetch" \
  "$BIN" credentials --check

section "IDC: assume"

for key in "${!SERVICE_ACCOUNTS[@]}"; do
  acct_env="${key%%-*}"
  acct_domain="${key#*-}"
  expected="${SERVICE_ACCOUNTS[$key]}"
  prefix="IDC: assume $acct_env/$acct_domain"

  rm -f ~/.quikstrate/"${acct_env}-${acct_domain}"-*-idc.json 2>/dev/null || true

  err=$(mktemp)
  json=$("$BIN" assume -e "$acct_env" -d "$acct_domain" $ROLE_FLAG --format json 2>"$err") || {
    fail "$prefix — command failed — $(tail -2 "$err" | tr '\n' ' ')"; rm -f "$err"; continue
  }
  rm -f "$err"
  echo "$json" | python3 -m json.tool > /dev/null 2>&1 || { fail "$prefix — invalid JSON"; continue; }

  actual=$(aws_account_for_creds "$json") || { fail "$prefix — sts failed"; continue; }
  if [[ "$actual" == "$expected" ]]; then
    pass "$prefix — correct account ($actual)"
  else
    fail "$prefix — wrong account: got $actual, expected $expected"
  fi

  idc_files=$(ls ~/.quikstrate/"${acct_env}-${acct_domain}"-*-idc.json 2>/dev/null || true)
  [[ -n "$idc_files" ]] \
    && pass "$prefix — cache file has -idc.json suffix" \
    || fail "$prefix — no -idc.json cache file written"

  plain_files=$(ls ~/.quikstrate/"${acct_env}-${acct_domain}"-*.json 2>/dev/null \
    | rg -v '\-idc\.json' || true)
  [[ -z "$plain_files" ]] \
    && pass "$prefix — Substrate cache file not written" \
    || fail "$prefix — unexpectedly wrote Substrate cache: $plain_files"
done

assert_fail "IDC: assume staging/internal-services exits non-zero" \
  "$BIN" assume -e staging -d internal-services

section "IDC: role normalization"

if [[ "$ENGINEERSREADONLY" == "true" ]]; then
  skip "IDC: --role Administrator normalizes to admin (pass without --engineersreadonly)"
else
  err=$(mktemp)
  norm_json=$("$BIN" assume -e staging -d api --role Administrator --format json 2>"$err") || true
  if echo "$norm_json" | python3 -m json.tool > /dev/null 2>&1; then
    norm_acct=$(aws_account_for_creds "$norm_json")
    [[ "$norm_acct" == "407752757973" ]] \
      && pass "IDC: --role Administrator normalizes to admin (407752757973)" \
      || fail "IDC: --role Administrator — wrong account $norm_acct"
  else
    fail "IDC: --role Administrator returned invalid JSON — $(tail -2 "$err" | tr '\n' ' ')"
  fi
  rm -f "$err"
fi

err=$(mktemp)
aud_json=$("$BIN" assume -e prod -d api --role Auditor --format json 2>"$err") || true
if echo "$aud_json" | python3 -m json.tool > /dev/null 2>&1; then
  aud_acct=$(aws_account_for_creds "$aud_json")
  [[ "$aud_acct" == "477056945755" ]] \
    && pass "IDC: --role Auditor normalizes to engineersreadonly (477056945755)" \
    || fail "IDC: --role Auditor — wrong account $aud_acct"
else
  fail "IDC: --role Auditor returned invalid JSON — $(tail -2 "$err" | tr '\n' ' ')"
fi
rm -f "$err"

section "IDC: special accounts"

for name in "${!SPECIAL_ACCOUNTS[@]}"; do
  expected="${SPECIAL_ACCOUNTS[$name]}"
  spec_json=$("$BIN" assume --special "$name" $ROLE_FLAG --format json 2>/dev/null) || {
    fail "IDC: assume --special $name — command failed"; continue
  }
  actual=$(aws_account_for_creds "$spec_json") || {
    fail "IDC: assume --special $name — sts failed"; continue
  }
  [[ "$actual" == "$expected" ]] \
    && pass "IDC: assume --special $name — correct account ($actual)" \
    || fail "IDC: assume --special $name — wrong account: got $actual, expected $expected"
done

section "IDC: accounts and whoami"

assert_ok   "IDC: accounts --format text exits 0" "$BIN" accounts --format text
assert_json "IDC: accounts --format json is valid JSON" "$BIN" accounts --format json
assert_output "IDC: accounts --format json has Accounts array" '"Accounts"' \
  "$BIN" accounts --format json

assert_ok   "IDC: whoami --format text exits 0" "$BIN" whoami --format text
assert_json "IDC: whoami --format json is valid JSON" "$BIN" whoami --format json
IDC_WHOAMI=$("$BIN" whoami --format json 2>/dev/null)
for field in AccountID Domain Environment Quality Role User; do
  val=$(echo "$IDC_WHOAMI" | json_field "$field")
  [[ -n "$val" ]] && pass "IDC: whoami has $field" || fail "IDC: whoami missing $field"
done

# ── Section 4: USE_SUBSTRATE=true escape hatch ───────────────────────────────

section "USE_SUBSTRATE=true escape hatch (IDC configured, forcing Substrate)"

idc_clear_creds
rm -f ~/.quikstrate/credentials.json

env USE_SUBSTRATE=true "$BIN" credentials --force --format json > /dev/null 2>&1
assert_ok "Escape hatch: credentials.json written when USE_SUBSTRATE=true overrides IDC config" \
  test -f ~/.quikstrate/credentials.json

if [[ -f ~/.quikstrate/credentials-idc.json ]]; then
  creds_mtime=$(stat -f %m ~/.quikstrate/credentials.json 2>/dev/null || stat -c %Y ~/.quikstrate/credentials.json 2>/dev/null)
  idc_mtime=$(stat -f %m ~/.quikstrate/credentials-idc.json 2>/dev/null || stat -c %Y ~/.quikstrate/credentials-idc.json 2>/dev/null)
  [[ "$creds_mtime" -ge "$idc_mtime" ]] \
    && pass "Escape hatch: credentials-idc.json not freshly written when USE_SUBSTRATE=true" \
    || fail "Escape hatch: credentials-idc.json appears freshly written despite USE_SUBSTRATE=true"
else
  pass "Escape hatch: credentials-idc.json does not exist"
fi

# ── Section 5: configure --use-substrate (revert) ────────────────────────────

section "configure --use-substrate (revert to Substrate)"

assert_ok "configure --use-substrate exits 0" "$BIN" configure --use-substrate

cfg_after=$(python3 -c "import json; print(json.load(open('$HOME/.quikstrate/config.json')).get('credential_source',''))")
[[ "$cfg_after" == "substrate" ]] \
  && pass "config.json reverted to credential_source=substrate" \
  || fail "config.json has unexpected credential_source=$cfg_after after --use-substrate"

idc_clear_creds
rm -f ~/.quikstrate/credentials.json
"$BIN" credentials --force --format json > /dev/null 2>&1
assert_ok "After --use-substrate, credentials writes credentials.json" \
  test -f ~/.quikstrate/credentials.json

IDC_MTIME=$(stat -f %m ~/.quikstrate/credentials-idc.json 2>/dev/null \
  || stat -c %Y ~/.quikstrate/credentials-idc.json 2>/dev/null || echo 0)
SUB_MTIME=$(stat -f %m ~/.quikstrate/credentials.json 2>/dev/null \
  || stat -c %Y ~/.quikstrate/credentials.json 2>/dev/null || echo 1)
[[ "$IDC_MTIME" -lt "$SUB_MTIME" ]] \
  && pass "credentials-idc.json not freshly written after configure --use-substrate" \
  || fail "credentials-idc.json appears freshly written after configure --use-substrate"

# ── summary ───────────────────────────────────────────────────────────────────
echo
echo "════════════════════════════════════════════════"
printf "  PASS: %d   FAIL: %d   SKIP: %d\n" "$PASS" "$FAIL" "$SKIP"
echo "════════════════════════════════════════════════"

if [[ ${#FAILURES[@]} -gt 0 ]]; then
  echo
  echo "Failed tests:"
  for f in "${FAILURES[@]}"; do
    echo "  - $f"
  done
  exit 1
fi

exit 0
