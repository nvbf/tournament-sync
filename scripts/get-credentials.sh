#!/bin/bash

# Get Firebase/Firestore credentials from Google Secret Manager
# Usage: ./scripts/get-credentials.sh [dev|prod]

set -euo pipefail

ENVIRONMENT=${1:-dev}

if [ "$ENVIRONMENT" != "dev" ] && [ "$ENVIRONMENT" != "prod" ]; then
  echo "Usage: $0 [dev|prod]"
  exit 1
fi

# Secrets are stored in osvb-scoreboard project
gcloud config set project osvb-scoreboard

SECRET_NAME="tournament-sync-firestore-credentials-${ENVIRONMENT}"

echo "Fetching $ENVIRONMENT credentials from secret: $SECRET_NAME"
echo ""

export FIRESTORE_CREDENTIAL_JSON=$(gcloud secrets versions access latest --secret="$SECRET_NAME")

echo "FIRESTORE_CREDENTIAL_JSON has been exported to environment variable"
echo ""
echo "You can now use it in your application:"
echo "  - In environment: \$FIRESTORE_CREDENTIAL_JSON"
echo "  - Or save it: echo \$FIRESTORE_CREDENTIAL_JSON > credentials.json"
echo ""
echo "To verify the credentials were loaded:"
echo "  echo \$FIRESTORE_CREDENTIAL_JSON | jq ."
