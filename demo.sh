#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEN_DIR="${ROOT_DIR}/generated"
POLICIES_JSON="${GEN_DIR}/policies.json"
TF_DIR="${ROOT_DIR}/terraform"
TF_VARS_JSON="${TF_DIR}/generated_policies.auto.tfvars.json"

echo "[demo] ensuring generated directory exists"
mkdir -p "${GEN_DIR}"

if [ ! -f "${POLICIES_JSON}" ]; then
  echo "[demo] policies.json not found; running Go renderer with discovery"

  go run "${ROOT_DIR}/cmd/renderer" \
    -discover \
    -region "us-west-2" \
    -config "${ROOT_DIR}/configs/policy-variants.yaml" \
    -output "${POLICIES_JSON}"
else
  echo "[demo] found existing ${POLICIES_JSON}; skipping discovery/render"
fi

echo "[demo] wrapping policies.json into Terraform tfvars"

cat > "${TF_VARS_JSON}" <<EOF
{
  "fms_policies_json": $(cat "${POLICIES_JSON}")
}
EOF

echo "[demo] applying Terraform configuration with rendered FMS policies"

cd "${TF_DIR}"
terraform init -input=false
terraform apply -auto-approve

echo "[demo] done. Check the outputs above and the AWS Console (Firewall Manager + ALB)."
