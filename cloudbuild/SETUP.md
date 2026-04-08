# Cloud Build and Cloud Run setup

This setup implements the following flow for project osvb-scoreboard and repository nvbf/tournament-sync:

1. Merge to main:
- Build and push image tagged build-$COMMIT_SHA.
- Publish a Pub/Sub event.
- Pub/Sub trigger deploys the image to dev.

2. Tag matching r[0-9]+:
- Re-tag previously built image build-$COMMIT_SHA as the release tag.
- Artifact Registry publishes tag events to native topic gcr.
- Pub/Sub trigger on gcr deploys to beta.
- Pub/Sub trigger on gcr creates prod deploy build that requires approval.

## Build config files

- cloudbuild/main-build.yaml
- cloudbuild/dev-deploy.yaml
- cloudbuild/release-dispatch.yaml
- cloudbuild/beta-deploy.yaml
- cloudbuild/prod-deploy.yaml

## Prerequisites

- Cloud Build, Cloud Run, Artifact Registry, Pub/Sub APIs enabled.
- Cloud Build connected to GitHub repo nvbf/tournament-sync.
- Artifact Registry docker repository created (default in YAML: tournament-sync).
- Artifact Registry notifications are publishing to topic gcr in this project.
- Cloud Run services created or deployable:
  - tournament-sync-dev
  - tournament-sync-beta
  - tournament-sync-prod

## One-command bootstrap

You can create required resources and triggers with:

```bash
./setup.sh
```

Optional overrides:

```bash
PROJECT_ID=osvb-scoreboard \
REPO_OWNER=nvbf \
REPO_NAME=tournament-sync \
REGION=europe-west1 \
TRIGGER_MODE=upsert \
BUILD_SERVICE_ACCOUNT_NAME=tournament-sync-build \
RUNTIME_SERVICE_ACCOUNT_NAME=tournament-sync-runtime \
FIRESTORE_PROJECT_ID_ENV=osvb-scoreboard \
PROFIXIO_HOST=https://replace-me.example.com \
FIRESTORE_DATABASE_ID='(default)' \
CORS_HOSTS='https://app.example.com,https://admin.example.com' \
HOST_URL=https://api.example.com \
FIRESTORE_CREDENTIAL_SECRET_DEV=tournament-sync-firestore-credentials-dev \
FIRESTORE_CREDENTIAL_SECRET_PROD=tournament-sync-firestore-credentials-prod \
APPROVER_PRINCIPAL=user:you@example.com \
./setup.sh
```

`TRIGGER_MODE` values:
- `create-only` (default): keep existing triggers unchanged and only create missing ones.
- `upsert`: delete and recreate existing triggers so config changes are applied.

Service account behavior:
- `setup.sh` creates two service accounts automatically:
  - build SA: `${BUILD_SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com`
  - runtime SA: `${RUNTIME_SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com`
- You can override full emails using `BUILD_SERVICE_ACCOUNT` and `RUNTIME_SERVICE_ACCOUNT`.

Runtime env behavior:
- Deploy pipelines set these Cloud Run environment variables required by `main.go`:
  - `FIRESTORE_PROJECT_ID`
  - `PROFIXIO_HOST`
  - `FIRESTORE_DATABASE_ID`
  - `CORS_HOSTS`
  - `HOST_URL`
- `FIRESTORE_CREDENTIAL_JSON` is injected from Secret Manager using `--set-secrets`.
- Secret mapping by environment:
  - dev: `FIRESTORE_CREDENTIAL_SECRET_DEV`
  - beta: `FIRESTORE_CREDENTIAL_SECRET_BETA` (defaults to dev secret)
  - prod: `FIRESTORE_CREDENTIAL_SECRET_PROD`
- `PORT` is not set manually; Cloud Run provides it automatically.

## Variables used in this setup

- PROJECT_ID: osvb-scoreboard
- REPO_OWNER: nvbf
- REPO_NAME: tournament-sync

Optional overrides in trigger substitutions:

- _ARTIFACT_REGION (default: europe-west1)
- _AR_REPO (default: tournament-sync)
- _IMAGE_NAME (default: tournament-sync)
- _BUILD_SERVICE_ACCOUNT (set by setup.sh)
- _RUN_REGION (default: europe-west1)

## Create Pub/Sub topics

Run:

```bash
gcloud config set project osvb-scoreboard

gcloud pubsub topics create tournament-sync-dev-deploy
```

For beta/prod, this setup uses native Artifact Registry events from topic gcr.

## Trigger matrix

1. Trigger: main-build
- Type: Repository event trigger (push)
- Branch regex: ^main$
- Config: cloudbuild/main-build.yaml
- Purpose: Build and push build-$COMMIT_SHA, then publish dev deploy event.

2. Trigger: dev-deploy-pubsub
- Type: Pub/Sub
- Topic: tournament-sync-dev-deploy
- Source: same repository, branch ^main$
- Config: cloudbuild/dev-deploy.yaml
- Substitution from payload:
  - _IMAGE_TAG = $(body.message.attributes.image_tag)
  - _SERVICE_NAME = $(body.message.attributes.service)

3. Trigger: release-dispatch
- Type: Repository event trigger (push tag)
- Tag regex: ^r[0-9]+$
- Config: cloudbuild/release-dispatch.yaml
- Purpose: Tag image build-$COMMIT_SHA as release tag.

4. Trigger: beta-deploy-pubsub
- Type: Pub/Sub
- Topic: gcr
- Source: same repository, branch ^main$
- Config: cloudbuild/beta-deploy.yaml
- Substitution from payload:
  - _IMAGE_TAG = $(body.message.data.tag)
  - _ACTION = $(body.message.data.action)
  - _SERVICE_NAME = tournament-sync-beta

5. Trigger: prod-deploy-pubsub
- Type: Pub/Sub
- Topic: gcr
- Source: same repository, branch ^main$
- Config: cloudbuild/prod-deploy.yaml
- Substitution from payload:
  - _IMAGE_TAG = $(body.message.data.tag)
  - _ACTION = $(body.message.data.action)
  - _SERVICE_NAME = tournament-sync-prod
- Approval: enable Require approval in trigger settings.

## Required IAM for build service account

Grant the build service account these roles:

- roles/run.admin
- roles/artifactregistry.writer
- roles/pubsub.publisher
- roles/iam.serviceAccountUser (on runtime service account used by Cloud Run)

For approvers of prod builds, grant:

- roles/cloudbuild.builds.approver

Runtime service account requirements:

- roles/secretmanager.secretAccessor (to read FIRESTORE_CREDENTIAL_JSON secret)

## Notes

- Prod approval is configured at trigger level, not inside YAML.
- release-dispatch uses TAG_NAME and COMMIT_SHA from the tag event.
- beta/prod deploy configs self-skip unless action is INSERT and image tag matches ^r[0-9]+$.
- If tag does not match r[0-9]+, release-dispatch fails by design.
