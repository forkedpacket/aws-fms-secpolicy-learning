#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
LAMBDA_BUILD_DIR="${DIST_DIR}/lambda"
LAMBDA_ZIP="${DIST_DIR}/lambda.zip"
TF_DIR="${ROOT_DIR}/terraform"

echo "[demo] building Lambda (linux/amd64) into ${LAMBDA_ZIP}"
mkdir -p "${LAMBDA_BUILD_DIR}"
GOOS=linux GOARCH=amd64 go build -o "${LAMBDA_BUILD_DIR}/main" "${ROOT_DIR}/cmd/lambda"
(cd "${LAMBDA_BUILD_DIR}" && zip -q ../lambda.zip main)

echo "[demo] applying Terraform (ALB without WAF + IAM + Lambda)"
cd "${TF_DIR}"
terraform init -input=false
terraform apply -auto-approve -var "lambda_package=${LAMBDA_ZIP}"

echo "[demo] done. Invoke the Lambda with {\"dryRun\":true} to see intended policy changes."
