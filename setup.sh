#!/usr/bin/env bash
set -euo pipefail

# End-to-end bootstrap for Cloud Build + Cloud Run delivery pipelines.
# Defaults are set for this repository but can be overridden via env vars.

PROJECT_ID="${PROJECT_ID:-osvb-scoreboard}"
REPO_OWNER="${REPO_OWNER:-nvbf}"
REPO_NAME="${REPO_NAME:-tournament-sync}"
REPO_DEFAULT_BRANCH="${REPO_DEFAULT_BRANCH:-main}"

REGION="${REGION:-europe-west1}"
TRIGGER_REGION="${TRIGGER_REGION:-${REGION}}"

AR_REPO="${AR_REPO:-tournament-sync}"
IMAGE_NAME="${IMAGE_NAME:-tournament-sync}"

DEV_SERVICE="${DEV_SERVICE:-tournament-sync-dev}"
BETA_SERVICE="${BETA_SERVICE:-tournament-sync-beta}"
PROD_SERVICE="${PROD_SERVICE:-tournament-sync-prod}"

FIRESTORE_PROJECT_ID_ENV="${FIRESTORE_PROJECT_ID_ENV:-${PROJECT_ID}}"
PROFIXIO_HOST="${PROFIXIO_HOST:-https://replace-me.example.com}"
FIRESTORE_DATABASE_ID="${FIRESTORE_DATABASE_ID:-(default)}"
CORS_HOSTS="${CORS_HOSTS:-https://replace-me.example.com}"
HOST_URL="${HOST_URL:-https://replace-me.example.com}"
FIRESTORE_CREDENTIAL_SECRET_DEV="${FIRESTORE_CREDENTIAL_SECRET_DEV:-tournament-sync-firestore-credentials-dev}"
FIRESTORE_CREDENTIAL_SECRET_PROD="${FIRESTORE_CREDENTIAL_SECRET_PROD:-tournament-sync-firestore-credentials-prod}"
FIRESTORE_CREDENTIAL_SECRET_BETA="${FIRESTORE_CREDENTIAL_SECRET_BETA:-${FIRESTORE_CREDENTIAL_SECRET_PROD}}"
PROFIXIO_KEY_SECRET="${PROFIXIO_KEY_SECRET:-tournament-sync-profixio-key}"
RESEND_KEY_SECRET="${RESEND_KEY_SECRET:-tournament-sync-resend-key}"

BUILD_SERVICE_ACCOUNT_NAME="${BUILD_SERVICE_ACCOUNT_NAME:-${REPO_NAME}-build}"
RUNTIME_SERVICE_ACCOUNT_NAME="${RUNTIME_SERVICE_ACCOUNT_NAME:-${REPO_NAME}-runtime}"

BUILD_SERVICE_ACCOUNT="${BUILD_SERVICE_ACCOUNT:-${BUILD_SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com}"
RUNTIME_SERVICE_ACCOUNT="${RUNTIME_SERVICE_ACCOUNT:-${RUNTIME_SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com}"

GCR_TOPIC="${GCR_TOPIC:-projects/osvb-scoreboard/topics/gcr}"

# Optional: principal to grant Cloud Build Approver role to.
# Example: user:you@example.com or group:platform@example.com
APPROVER_PRINCIPAL="${APPROVER_PRINCIPAL:-}"

# Trigger behavior on rerun:
# - create-only: skip existing triggers (default)
# - upsert: delete and recreate existing triggers
TRIGGER_MODE="${TRIGGER_MODE:-upsert}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: ${cmd}" >&2
    exit 1
  fi
}

trigger_exists() {
  local name="$1"
  gcloud builds triggers list \
    --project="$PROJECT_ID" \
    --region="$TRIGGER_REGION" \
    --format='value(name)' | grep -Fxq "$name"
}

delete_trigger_by_name() {
  local name="$1"
  gcloud builds triggers delete "$name" \
    --project="$PROJECT_ID" \
    --region="$TRIGGER_REGION" \
    --quiet
}

create_or_reconcile_trigger() {
  local name="$1"
  shift

  if trigger_exists "$name"; then
    if [[ "$TRIGGER_MODE" == "upsert" ]]; then
      echo "Trigger exists, replacing: $name"
      delete_trigger_by_name "$name"
    else
      echo "Trigger already exists, skipping: $name"
      return
    fi
  fi

  echo "Creating trigger: $name"
  local create_output
  if ! create_output="$(gcloud "$@" 2>&1)"; then
    if echo "$create_output" | grep -Eiq 'already exists|ALREADY_EXISTS'; then
      echo "Trigger already exists, skipping: $name"
      return
    fi
    echo "$create_output" >&2
    return 1
  fi

  echo "$create_output"
}

ensure_service_account() {
  local email="$1"
  local display_name="$2"
  local account_id="${email%@*}"

  if gcloud iam service-accounts describe "$email" --project="$PROJECT_ID" >/dev/null 2>&1; then
    echo "Service account already exists: $email"
    return
  fi

  echo "Creating service account: $email"
  gcloud iam service-accounts create "$account_id" \
    --project="$PROJECT_ID" \
    --display-name="$display_name"
}

ensure_secret() {
  local name="$1"

  if gcloud secrets describe "$name" --project="$PROJECT_ID" >/dev/null 2>&1; then
    echo "Secret already exists: $name"
    return
  fi

  echo "Creating secret: $name"
  gcloud secrets create "$name" \
    --project="$PROJECT_ID" \
    --replication-policy=automatic
  echo "Secret created without versions: $name"
}

require_cmd gcloud

if [[ "$TRIGGER_MODE" != "create-only" && "$TRIGGER_MODE" != "upsert" ]]; then
  echo "Invalid TRIGGER_MODE: $TRIGGER_MODE (expected: create-only|upsert)" >&2
  exit 1
fi

cd "$ROOT_DIR"

echo "Using project: $PROJECT_ID"
gcloud config set project "$PROJECT_ID" >/dev/null

PROJECT_NUMBER="$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')"

echo "Ensuring dedicated service accounts exist..."
ensure_service_account "$BUILD_SERVICE_ACCOUNT" "${REPO_NAME} Cloud Build executor"
ensure_service_account "$RUNTIME_SERVICE_ACCOUNT" "${REPO_NAME} Cloud Run runtime"

echo "Enabling required APIs..."
gcloud services enable \
  cloudbuild.googleapis.com \
  logging.googleapis.com \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  pubsub.googleapis.com \
  secretmanager.googleapis.com

echo "Ensuring Artifact Registry repository exists: ${AR_REPO} (${REGION})"
if ! gcloud artifacts repositories describe "$AR_REPO" \
  --project="$PROJECT_ID" \
  --location="$REGION" >/dev/null 2>&1; then
  gcloud artifacts repositories create "$AR_REPO" \
    --project="$PROJECT_ID" \
    --location="$REGION" \
    --repository-format=docker \
    --description="Container images for ${REPO_NAME}"
else
  echo "Artifact Registry repository already exists: $AR_REPO"
fi

echo "Ensuring Artifact Registry notification topic exists..."
if gcloud pubsub topics describe "$GCR_TOPIC" --project="$PROJECT_ID" >/dev/null 2>&1; then
  echo "Found Artifact Registry topic: $GCR_TOPIC"
else
  echo "Creating Artifact Registry topic: $GCR_TOPIC"
  gcloud pubsub topics create "$GCR_TOPIC" --project="$PROJECT_ID"
fi

echo "Granting IAM roles to build service account: $BUILD_SERVICE_ACCOUNT"
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${BUILD_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/run.admin" >/dev/null

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${BUILD_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/artifactregistry.writer" >/dev/null

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${BUILD_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/pubsub.publisher" >/dev/null

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${BUILD_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/logging.logWriter" >/dev/null

echo "Granting iam.serviceAccountUser on runtime service account to build service account"
gcloud iam service-accounts add-iam-policy-binding "$RUNTIME_SERVICE_ACCOUNT" \
  --project="$PROJECT_ID" \
  --member="serviceAccount:${BUILD_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/iam.serviceAccountUser" >/dev/null

echo "Granting runtime service account access to Secret Manager"
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${RUNTIME_SERVICE_ACCOUNT}" --condition=None \
  --role="roles/secretmanager.secretAccessor" >/dev/null

echo "Ensuring Firestore credential secrets exist (dev/prod)..."
ensure_secret "$FIRESTORE_CREDENTIAL_SECRET_DEV"
ensure_secret "$FIRESTORE_CREDENTIAL_SECRET_PROD"
echo "Ensuring Profixio and Resend key secrets exist..."
ensure_secret "$PROFIXIO_KEY_SECRET"
ensure_secret "$RESEND_KEY_SECRET"

for secret_name in \
  "$FIRESTORE_CREDENTIAL_SECRET_DEV" "$FIRESTORE_CREDENTIAL_SECRET_PROD" "$FIRESTORE_CREDENTIAL_SECRET_BETA" \
  "$PROFIXIO_KEY_SECRET" \
  "$RESEND_KEY_SECRET"; do
  if gcloud secrets versions list "$secret_name" --project="$PROJECT_ID" --format='value(name)' --limit=1 | grep -q .; then
    echo "Secret has at least one version: $secret_name"
  else
    echo "Warning: secret '$secret_name' has no versions yet."
    echo "Add a version before deploying:"
    echo "  gcloud secrets versions add $secret_name --data-file=path/to/firestore-credentials.json"
  fi
done

if [[ -n "$APPROVER_PRINCIPAL" ]]; then
  echo "Granting Cloud Build Approver to: $APPROVER_PRINCIPAL"
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="$APPROVER_PRINCIPAL" --condition=None \
    --role="roles/cloudbuild.builds.approver" >/dev/null
else
  echo "No APPROVER_PRINCIPAL provided; skipping approver IAM grant."
fi

TR_MAIN_BUILD="${REPO_NAME}-main-build"
TR_RELEASE_DISPATCH="${REPO_NAME}-release-dispatch"
TR_DEV_DEPLOY="${REPO_NAME}-dev-deploy-pubsub"
TR_BETA_DEPLOY="${REPO_NAME}-beta-deploy-pubsub"
TR_PROD_DEPLOY="${REPO_NAME}-prod-deploy-pubsub"

echo "Creating repository event triggers (main + release tags)..."
create_or_reconcile_trigger "$TR_MAIN_BUILD" builds triggers create github \
  --project="$PROJECT_ID" \
  --name="$TR_MAIN_BUILD" \
  --service-account="projects/${PROJECT_ID}/serviceAccounts/${BUILD_SERVICE_ACCOUNT}" \
  --repo-owner="$REPO_OWNER" \
  --repo-name="$REPO_NAME" \
  --branch-pattern='^main$' \
  --build-config='cloudbuild/main-build.yaml' \
  --substitutions="_ARTIFACT_REGION=${REGION},_AR_REPO=${AR_REPO},_IMAGE_NAME=${IMAGE_NAME},_BUILD_SERVICE_ACCOUNT=${BUILD_SERVICE_ACCOUNT}"

create_or_reconcile_trigger "$TR_RELEASE_DISPATCH" builds triggers create github \
  --project="$PROJECT_ID" \
  --name="$TR_RELEASE_DISPATCH" \
  --service-account="projects/${PROJECT_ID}/serviceAccounts/${BUILD_SERVICE_ACCOUNT}" \
  --repo-owner="$REPO_OWNER" \
  --repo-name="$REPO_NAME" \
  --tag-pattern='^r[0-9]+$' \
  --build-config='cloudbuild/release-dispatch.yaml' \
  --substitutions="_ARTIFACT_REGION=${REGION},_AR_REPO=${AR_REPO},_IMAGE_NAME=${IMAGE_NAME},_BUILD_SERVICE_ACCOUNT=${BUILD_SERVICE_ACCOUNT}"

echo "Creating Pub/Sub deploy triggers (dev, beta, prod)..."
create_or_reconcile_trigger "$TR_DEV_DEPLOY" builds triggers create pubsub \
  --project="$PROJECT_ID" \
  --name="$TR_DEV_DEPLOY" \
  --service-account="projects/${PROJECT_ID}/serviceAccounts/${BUILD_SERVICE_ACCOUNT}" \
  --repo="https://github.com/${REPO_OWNER}/${REPO_NAME}" \
  --repo-type="GITHUB" \
  --branch="$REPO_DEFAULT_BRANCH" \
  --topic="$GCR_TOPIC" \
  --subscription-filter="_ACTION == \"INSERT\" && _IMAGE_TAG.matches(\"^${REGION}-docker\\\\.pkg\\\\.dev/${PROJECT_ID}/${AR_REPO}/${IMAGE_NAME}:build-[0-9a-f]+$\")" \
  --build-config='cloudbuild/dev-deploy.yaml' \
  --substitutions="_ARTIFACT_REGION=${REGION},_AR_REPO=${AR_REPO},_IMAGE_NAME=${IMAGE_NAME},_BUILD_SERVICE_ACCOUNT=${BUILD_SERVICE_ACCOUNT},_RUN_REGION=${REGION},_RUNTIME_SERVICE_ACCOUNT=${RUNTIME_SERVICE_ACCOUNT},_FIRESTORE_PROJECT_ID=${FIRESTORE_PROJECT_ID_ENV},_PROFIXIO_HOST=${PROFIXIO_HOST},_FIRESTORE_DATABASE_ID=${FIRESTORE_DATABASE_ID},_CORS_HOSTS=${CORS_HOSTS},_HOST_URL=${HOST_URL},_FIRESTORE_CREDENTIAL_SECRET=${FIRESTORE_CREDENTIAL_SECRET_DEV},_PROFIXIO_KEY_SECRET=${PROFIXIO_KEY_SECRET},_RESEND_KEY_SECRET=${RESEND_KEY_SECRET},_SERVICE_NAME=${DEV_SERVICE},_IMAGE_TAG=\$(body.message.data.tag),_ACTION=\$(body.message.data.action)"

create_or_reconcile_trigger "$TR_BETA_DEPLOY" builds triggers create pubsub \
  --project="$PROJECT_ID" \
  --name="$TR_BETA_DEPLOY" \
  --service-account="projects/${PROJECT_ID}/serviceAccounts/${BUILD_SERVICE_ACCOUNT}" \
  --repo="https://github.com/${REPO_OWNER}/${REPO_NAME}" \
  --repo-type="GITHUB" \
  --branch="$REPO_DEFAULT_BRANCH" \
  --topic="$GCR_TOPIC" \
  --subscription-filter="_ACTION == \"INSERT\" && _IMAGE_TAG.matches(\"^${REGION}-docker\\\\.pkg\\\\.dev/${PROJECT_ID}/${AR_REPO}/${IMAGE_NAME}:r[0-9]+$\")" \
  --build-config='cloudbuild/beta-deploy.yaml' \
  --substitutions="_ARTIFACT_REGION=${REGION},_AR_REPO=${AR_REPO},_IMAGE_NAME=${IMAGE_NAME},_BUILD_SERVICE_ACCOUNT=${BUILD_SERVICE_ACCOUNT},_RUN_REGION=${REGION},_RUNTIME_SERVICE_ACCOUNT=${RUNTIME_SERVICE_ACCOUNT},_FIRESTORE_PROJECT_ID=${FIRESTORE_PROJECT_ID_ENV},_PROFIXIO_HOST=${PROFIXIO_HOST},_FIRESTORE_DATABASE_ID=${FIRESTORE_DATABASE_ID},_CORS_HOSTS=${CORS_HOSTS},_HOST_URL=${HOST_URL},_FIRESTORE_CREDENTIAL_SECRET=${FIRESTORE_CREDENTIAL_SECRET_BETA},_PROFIXIO_KEY_SECRET=${PROFIXIO_KEY_SECRET},_RESEND_KEY_SECRET=${RESEND_KEY_SECRET},_SERVICE_NAME=${BETA_SERVICE},_IMAGE_TAG=\$(body.message.data.tag),_ACTION=\$(body.message.data.action)"

create_or_reconcile_trigger "$TR_PROD_DEPLOY" builds triggers create pubsub \
  --project="$PROJECT_ID" \
  --name="$TR_PROD_DEPLOY" \
  --service-account="projects/${PROJECT_ID}/serviceAccounts/${BUILD_SERVICE_ACCOUNT}" \
  --repo="https://github.com/${REPO_OWNER}/${REPO_NAME}" \
  --repo-type="GITHUB" \
  --branch="$REPO_DEFAULT_BRANCH" \
  --topic="$GCR_TOPIC" \
  --subscription-filter="_ACTION == \"INSERT\" && _IMAGE_TAG.matches(\"^${REGION}-docker\\\\.pkg\\\\.dev/${PROJECT_ID}/${AR_REPO}/${IMAGE_NAME}:r[0-9]+$\")" \
  --build-config='cloudbuild/prod-deploy.yaml' \
  --require-approval \
  --substitutions="_ARTIFACT_REGION=${REGION},_AR_REPO=${AR_REPO},_IMAGE_NAME=${IMAGE_NAME},_BUILD_SERVICE_ACCOUNT=${BUILD_SERVICE_ACCOUNT},_RUN_REGION=${REGION},_RUNTIME_SERVICE_ACCOUNT=${RUNTIME_SERVICE_ACCOUNT},_FIRESTORE_PROJECT_ID=${FIRESTORE_PROJECT_ID_ENV},_PROFIXIO_HOST=${PROFIXIO_HOST},_FIRESTORE_DATABASE_ID=${FIRESTORE_DATABASE_ID},_CORS_HOSTS=${CORS_HOSTS},_HOST_URL=${HOST_URL},_FIRESTORE_CREDENTIAL_SECRET=${FIRESTORE_CREDENTIAL_SECRET_PROD},_PROFIXIO_KEY_SECRET=${PROFIXIO_KEY_SECRET},_RESEND_KEY_SECRET=${RESEND_KEY_SECRET},_SERVICE_NAME=${PROD_SERVICE},_IMAGE_TAG=\$(body.message.data.tag),_ACTION=\$(body.message.data.action)"

echo
echo "Setup completed."
echo "Project: $PROJECT_ID"
echo "Region: $REGION"
echo "Build service account: $BUILD_SERVICE_ACCOUNT"
echo "Runtime service account: $RUNTIME_SERVICE_ACCOUNT"
echo "Trigger mode: $TRIGGER_MODE"
echo "Repo trigger source: ${REPO_OWNER}/${REPO_NAME}"
echo "If this is your first run, verify Cloud Build GitHub app/repository connection is already configured."
