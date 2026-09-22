#!/usr/bin/env bash
# Keeps the Helm charts in sync with what controller-gen just wrote to
# config/crd/bases and config/rbac/role.yaml, so they can never silently
# drift the way they repeatedly have in the past:
#   - helm/site-operator-crds/templates/*.yaml mirrors config/crd/bases/*.yaml
#     1:1 (same basenames), so a removed/renamed CRD is reflected here too.
#   - helm/site-operator/templates/rbac/clusterrole.yaml's `rules:` are
#     regenerated from config/rbac/role.yaml's `rules:`, keeping this file's
#     own apiVersion/kind/metadata (name stays "site-operator", not
#     "manager-role").
#
# Run via `make manifests` (which calls this automatically) rather than by
# hand.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CRD_SRC_DIR="config/crd/bases"
CRD_DST_DIR="helm/site-operator-crds/templates"
RBAC_SRC="config/rbac/role.yaml"
RBAC_DST="helm/site-operator/templates/rbac/clusterrole.yaml"

echo "syncing CRDs: ${CRD_SRC_DIR} -> ${CRD_DST_DIR}"
rm -f "${CRD_DST_DIR}"/*.yaml
for f in "${CRD_SRC_DIR}"/*.yaml; do
  cp "$f" "${CRD_DST_DIR}/$(basename "$f")"
done

echo "syncing RBAC rules: ${RBAC_SRC} -> ${RBAC_DST}"
{
  echo "apiVersion: rbac.authorization.k8s.io/v1"
  echo "kind: ClusterRole"
  echo "metadata:"
  echo "  name: site-operator"
  sed -n '/^rules:/,$p' "${RBAC_SRC}"
} > "${RBAC_DST}"
