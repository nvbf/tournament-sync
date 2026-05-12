#!/bin/bash

# Print a Firebase ID token for API testing.
# Usage: ./scripts/get-auth-token.sh [dev|prod]

set -euo pipefail

ENVIRONMENT=${1:-dev}

if [[ "$ENVIRONMENT" != "dev" && "$ENVIRONMENT" != "prod" ]]; then
  echo "Usage: $0 [dev|prod]" >&2
  exit 1
fi

if ! command -v gcloud >/dev/null 2>&1; then
  echo "Error: gcloud is required" >&2
  exit 1
fi

if ! command -v firebase >/dev/null 2>&1; then
  echo "Error: firebase CLI is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required" >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "Error: openssl is required" >&2
  exit 1
fi

# Secrets live in this project.
gcloud config set project osvb-scoreboard >/dev/null

# Fetch service-account credentials JSON from Secret Manager.
FIRESTORE_CREDENTIAL_JSON=$(gcloud secrets versions access latest --secret="tournament-sync-firestore-credentials-${ENVIRONMENT}")
PROJECT_ID=$(jq -r '.project_id' <<<"$FIRESTORE_CREDENTIAL_JSON")
SERVICE_ACCOUNT_EMAIL=$(jq -r '.client_email' <<<"$FIRESTORE_CREDENTIAL_JSON")
PRIVATE_KEY=$(jq -r '.private_key' <<<"$FIRESTORE_CREDENTIAL_JSON")

if [[ -z "$PROJECT_ID" || "$PROJECT_ID" == "null" ]]; then
  echo "Error: could not read project_id from credentials" >&2
  exit 1
fi

if [[ -z "$SERVICE_ACCOUNT_EMAIL" || "$SERVICE_ACCOUNT_EMAIL" == "null" ]]; then
  echo "Error: could not read client_email from credentials" >&2
  exit 1
fi

if [[ -z "$PRIVATE_KEY" || "$PRIVATE_KEY" == "null" ]]; then
  echo "Error: could not read private_key from credentials" >&2
  exit 1
fi

TMP_CREDS=$(mktemp)
TMP_KEY=$(mktemp)
trap 'rm -f "$TMP_CREDS" "$TMP_KEY"' EXIT
printf '%s' "$FIRESTORE_CREDENTIAL_JSON" > "$TMP_CREDS"
printf '%s\n' "$PRIVATE_KEY" > "$TMP_KEY"

# Find a WEB app in the Firebase project, then read its API key from sdkconfig.
WEB_APP_ID=$(firebase apps:list --project "$PROJECT_ID" --json | jq -r '.result[] | select(.platform == "WEB") | .appId' | head -n1)
if [[ -z "$WEB_APP_ID" || "$WEB_APP_ID" == "null" ]]; then
  echo "Error: no Firebase WEB app found in project $PROJECT_ID" >&2
  exit 1
fi

WEB_API_KEY=$(firebase apps:sdkconfig WEB "$WEB_APP_ID" --project "$PROJECT_ID" --json | jq -r '.result.sdkConfig.apiKey')
if [[ -z "$WEB_API_KEY" || "$WEB_API_KEY" == "null" ]]; then
  echo "Error: could not resolve Firebase Web API key for project $PROJECT_ID" >&2
  exit 1
fi

# Create a Firebase custom token by signing a JWT locally, then exchange for an ID token.
NOW=$(date +%s)
EXP=$((NOW + 3600))

JWT_HEADER='{"alg":"RS256","typ":"JWT"}'
JWT_PAYLOAD=$(jq -nc \
  --arg iss "$SERVICE_ACCOUNT_EMAIL" \
  --arg sub "$SERVICE_ACCOUNT_EMAIL" \
  --arg aud "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit" \
  --arg uid "anon-local" \
  --argjson iat "$NOW" \
  --argjson exp "$EXP" \
  '{iss:$iss,sub:$sub,aud:$aud,uid:$uid,iat:$iat,exp:$exp}')

base64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

JWT_HEADER_B64=$(printf '%s' "$JWT_HEADER" | base64url)
JWT_PAYLOAD_B64=$(printf '%s' "$JWT_PAYLOAD" | base64url)
JWT_UNSIGNED="${JWT_HEADER_B64}.${JWT_PAYLOAD_B64}"
JWT_SIGNATURE_B64=$(printf '%s' "$JWT_UNSIGNED" | openssl dgst -sha256 -sign "$TMP_KEY" | base64url)

CUSTOM_TOKEN="${JWT_UNSIGNED}.${JWT_SIGNATURE_B64}"

if [[ -z "$CUSTOM_TOKEN" || "$CUSTOM_TOKEN" == "null" ]]; then
  echo "Error: failed to generate custom token" >&2
  exit 1
fi

ID_TOKEN=$(curl -sS -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken?key=${WEB_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"${CUSTOM_TOKEN}\",\"returnSecureToken\":true}" | jq -r '.idToken')

if [[ -z "$ID_TOKEN" || "$ID_TOKEN" == "null" ]]; then
  echo "Error: failed to exchange custom token for ID token" >&2
  exit 1
fi

echo "$ID_TOKEN"

