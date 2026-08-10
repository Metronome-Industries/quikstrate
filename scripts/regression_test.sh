#!/usr/bin/env bash
# A Claude generated regression test suite for quikstrate.
# Verifies behavioral contracts: credentials have the right shape and actually
# work against the expected AWS accounts. All AWS calls are read-only.
#
# Usage:
#   ./scripts/regression_test.sh [--run-configure]
#
#   --run-configure  Write real ~/.aws/config and ~/.kube/config, then verify
#                    a sample of AWS_PROFILE values work end-to-end.

set -euo pipefail

# ── flags ────────────────────────────────────────────────────────────────────
RUN_CONFIGURE=false
for arg in "$@"; do
  case "$arg" in
    --run-configure) RUN_CONFIGURE=true ;;
    *) echo "Unknown flag: $arg"; exit 1 ;;
  esac
done

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

section() { echo; echo "── $1 ──────────────────────────────────────────────"; }

# ── account sample ────────────────────────────────────────────────────────────
# Representative assume-role targets: staging, prod, and the prod-only domain.
# Source: metronome-substrate/substrate.accounts.txt
# Key format: "<env>-<domain>"  Value: expected AWS account ID
declare -A SERVICE_ACCOUNTS=(
  ["staging-api"]="407752757973"
  ["staging-graphql"]="008444403661"
  ["prod-api"]="477056945755"
  ["prod-graphql"]="051318803586"
  ["prod-internal-services"]="207567762512"  # prod-only; staging variant must fail
)

# Fields required by the AWS credential_process spec (fixed by AWS SDK, cannot change)
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

# ── configure ─────────────────────────────────────────────────────────────────
# Runs first when --run-configure is passed so all subsequent tests use the
# freshly written ~/.aws/config and ~/.kube/config.
section "configure"

assert_ok "configure --dryrun exits 0" "$BIN" configure --dryrun

if [[ "$RUN_CONFIGURE" == "true" ]]; then
  echo "  INFO  Writing ~/.aws/config and ~/.kube/config."
  assert_ok "configure exits 0" "$BIN" configure
  assert_ok "configure --check passes after configure" "$BIN" configure --check
else
  echo
  echo "  INFO  Pass --run-configure to write config files and test each AWS_PROFILE end-to-end."
fi

# ── credentials ──────────────────────────────────────────────────────────────
section "credentials"

CRED_JSON=$("$BIN" credentials --format json 2>/dev/null)

if echo "$CRED_JSON" | python3 -m json.tool > /dev/null 2>&1; then
  pass "credentials --format json is valid JSON"
else
  fail "credentials --format json is not valid JSON"
fi

for field in "${CRED_FIELDS[@]}"; do
  val=$(echo "$CRED_JSON" | json_field "$field")
  if [[ -n "$val" ]]; then
    pass "credentials --format json has $field"
  else
    fail "credentials --format json missing $field"
  fi
done

assert_output "credentials --format json Version is 1" '"Version": 1' printf '%s' "$CRED_JSON"

# export format must set the three standard AWS env vars (all on one line, possibly leading space)
CRED_EXPORT=$("$BIN" credentials --format export 2>/dev/null)
for varname in AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN; do
  if echo "$CRED_EXPORT" | rg -q "$varname="; then
    pass "credentials --format export sets $varname"
  else
    fail "credentials --format export missing $varname"
  fi
done

# Functional: credentials returned must be usable against AWS
KEY=$(echo "$CRED_JSON" | json_field AccessKeyId)
SECRET=$(echo "$CRED_JSON" | json_field SecretAccessKey)
TOKEN=$(echo "$CRED_JSON" | json_field SessionToken)
assert_ok "credentials work (sts get-caller-identity)" \
  env AWS_ACCESS_KEY_ID="$KEY" AWS_SECRET_ACCESS_KEY="$SECRET" AWS_SESSION_TOKEN="$TOKEN" \
  aws sts get-caller-identity

# --force fetches fresh credentials that also work
FORCE_JSON=$("$BIN" credentials --force --format json 2>/dev/null)
FORCE_KEY=$(echo "$FORCE_JSON" | json_field AccessKeyId)
FORCE_SECRET=$(echo "$FORCE_JSON" | json_field SecretAccessKey)
FORCE_TOKEN=$(echo "$FORCE_JSON" | json_field SessionToken)
assert_ok "credentials --force work (sts get-caller-identity)" \
  env AWS_ACCESS_KEY_ID="$FORCE_KEY" AWS_SECRET_ACCESS_KEY="$FORCE_SECRET" AWS_SESSION_TOKEN="$FORCE_TOKEN" \
  aws sts get-caller-identity

assert_ok "credentials --check exits 0 after fresh fetch" "$BIN" credentials --check

# ── assume ───────────────────────────────────────────────────────────────────
section "assume: flag validation"

assert_fail "assume with no flags exits non-zero" "$BIN" assume
assert_fail "assume --env only exits non-zero (missing --domain)" "$BIN" assume --env staging
assert_fail "assume --domain only exits non-zero (missing --env)" "$BIN" assume --domain api
assert_fail "assume with invalid --env exits non-zero" "$BIN" assume --env badenv --domain api
assert_fail "assume staging/internal-services exits non-zero (account does not exist)" \
  "$BIN" assume -e staging -d internal-services

# Test a single env/domain: validate credential shape and verify the right AWS account
test_assume() {
  local acct_env="$1" domain="$2" expected_account="$3"
  local prefix="assume $acct_env/$domain"
  local json

  json=$("$BIN" assume -e "$acct_env" -d "$domain" --format json 2>/dev/null) || {
    fail "$prefix — command failed"
    return
  }

  if ! echo "$json" | python3 -m json.tool > /dev/null 2>&1; then
    fail "$prefix — invalid JSON"
    return
  fi

  for field in "${CRED_FIELDS[@]}"; do
    val=$(echo "$json" | json_field "$field")
    if [[ -z "$val" ]]; then
      fail "$prefix — missing field $field"
    fi
  done

  local key secret token actual_account
  key=$(echo "$json" | json_field AccessKeyId)
  secret=$(echo "$json" | json_field SecretAccessKey)
  token=$(echo "$json" | json_field SessionToken)

  actual_account=$(AWS_ACCESS_KEY_ID="$key" AWS_SECRET_ACCESS_KEY="$secret" AWS_SESSION_TOKEN="$token" \
    aws sts get-caller-identity --query Account --output text 2>/dev/null) || {
    fail "$prefix — sts get-caller-identity failed with returned credentials"
    return
  }

  if [[ "$actual_account" == "$expected_account" ]]; then
    pass "$prefix — correct account ($actual_account)"
  else
    fail "$prefix — wrong account: got $actual_account, expected $expected_account"
  fi
}

section "assume (sample)"
for key in "${!SERVICE_ACCOUNTS[@]}"; do
  acct_env="${key%%-*}"
  acct_domain="${key#*-}"
  test_assume "$acct_env" "$acct_domain" "${SERVICE_ACCOUNTS[$key]}"
done

# ── accounts ─────────────────────────────────────────────────────────────────
section "accounts"

assert_ok   "accounts --format text exits 0" "$BIN" accounts --format text
assert_json "accounts --format json is valid JSON" "$BIN" accounts --format json
assert_output "accounts --format json has Accounts array" '"Accounts"' "$BIN" accounts --format json
assert_output "accounts --format json contains staging accounts" "staging" "$BIN" accounts --format json
assert_output "accounts --format json contains prod accounts" "prod" "$BIN" accounts --format json

# ── whoami ────────────────────────────────────────────────────────────────────
section "whoami"

assert_ok   "whoami --format text exits 0" "$BIN" whoami --format text
assert_json "whoami --format json is valid JSON" "$BIN" whoami --format json

WHOAMI_JSON=$("$BIN" whoami --format json 2>/dev/null)
for field in AccountID Domain Environment Quality Role User; do
  val=$(echo "$WHOAMI_JSON" | json_field "$field")
  if [[ -n "$val" ]]; then
    pass "whoami --format json has $field"
  else
    fail "whoami --format json missing or empty $field"
  fi
done

STS_ACCOUNT=$(aws sts get-caller-identity --query Account --output text 2>/dev/null)
WHOAMI_ACCOUNT=$(echo "$WHOAMI_JSON" | json_field AccountID)
if [[ "$STS_ACCOUNT" == "$WHOAMI_ACCOUNT" ]]; then
  pass "whoami AccountID matches sts get-caller-identity ($STS_ACCOUNT)"
else
  fail "whoami AccountID mismatch: whoami=$WHOAMI_ACCOUNT sts=$STS_ACCOUNT"
fi

# ── configure: AWS_PROFILE spot checks ───────────────────────────────────────
if [[ "$RUN_CONFIGURE" == "true" ]]; then
  section "configure: AWS_PROFILE spot checks"

  for profile in staging-api staging-graphql prod-api prod-graphql prod-internal-services; do
    if AWS_PROFILE="$profile" aws sts get-caller-identity > /dev/null 2>&1; then
      pass "AWS_PROFILE=$profile works (sts get-caller-identity)"
    else
      fail "AWS_PROFILE=$profile does not work"
    fi
  done

  for profile in management audit deploy network; do
    if AWS_PROFILE="$profile" aws sts get-caller-identity > /dev/null 2>&1; then
      pass "AWS_PROFILE=$profile works (sts get-caller-identity)"
    else
      fail "AWS_PROFILE=$profile does not work"
    fi
  done
fi

# ── clean ─────────────────────────────────────────────────────────────────────
section "clean"
echo "  INFO  Skipped to preserve credential cache from this run."
echo "        To test manually: quikstrate clean && quikstrate credentials"

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
