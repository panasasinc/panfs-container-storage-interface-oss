#!/bin/bash
# Copyright 2025 VDURA Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Purpose:
# Check that the charts, the manifests, the Makefile and the documentation all use one driver version.
#
# Usage:
#   check-version.sh [VERSION]
#
# With VERSION (X.Y.Z), every place must use that version; the Create Release workflow runs it this way.
# Without VERSION, the appVersion of charts/panfs/Chart.yaml is the expected version; CI runs it this way.
# The script reports every mismatch and exits with 1 if it finds one. Run it from the repository root.

set -euo pipefail

readonly DRIVER_CHART="charts/panfs/Chart.yaml"
readonly IMAGE_NAME="panfs-csi-driver"
readonly TENANT_NETWORK_IMAGE_NAME="panfs-tenant-network"
readonly STORAGECLASS_OPERATOR_IMAGE_NAME="panfs-storageclass-operator"

errors=0

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    errors=$((errors + 1))
}

# chart_field FILE FIELD - print the quoted value of a top-level field of a Chart.yaml
chart_field() {
    sed -n -E "s/^$2: \"([^\"]*)\"\$/\1/p" "$1"
}

# check_chart FILE - both version and appVersion must be the expected version
check_chart() {
    local field value
    for field in version appVersion; do
        value=$(chart_field "$1" "$field")
        if [[ "$value" != "$version" ]]; then
            fail "$1: $field is '${value:-<not found>}', expected '$version'"
        fi
    done
}

# check_image_tags FILE [IMAGE] - every image of that name in the file must have the expected tag (default: the driver image)
check_image_tags() {
    local image="${2:-$IMAGE_NAME}" tags tag
    tags=$(grep -o -E "${image}:[^[:space:]\"']+" "$1" | sed "s/^${image}://" | sort -u || true)
    if [[ -z "$tags" ]]; then
        fail "$1: no '${image}:<tag>' image found"
        return
    fi
    while IFS= read -r tag; do
        if [[ "$tag" != "$version" ]]; then
            fail "$1: tag of image ${image} is '$tag', expected '$version'"
        fi
    done <<<"$tags"
}

# check_contains FILE PATTERN DESCRIPTION - the file must have a line that matches the extended regex
check_contains() {
    if ! grep -q -E "$2" "$1"; then
        fail "$1: $3"
    fi
}

if [[ ! -f "$DRIVER_CHART" ]]; then
    printf 'ERROR: %s not found. Run this script from the repository root.\n' "$DRIVER_CHART" >&2
    exit 2
fi

version="${1:-$(chart_field "$DRIVER_CHART" appVersion)}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf "ERROR: version '%s' must match X.Y.Z\n" "$version" >&2
    exit 2
fi
readonly version
readonly version_re="${version//./\\.}"

printf 'Checking that everything uses driver version %s\n' "$version"

check_chart "$DRIVER_CHART"
check_chart "charts/storageclass/Chart.yaml"
check_chart "charts/tenant-network-operator/Chart.yaml"
check_chart "charts/tenant/Chart.yaml"
check_chart "charts/storageclass-operator/Chart.yaml"

for manifest in deploy/k8s/csi-driver/*.yaml examples/k8s/base/csi-panfs-driver.yaml; do
    check_image_tags "$manifest"
done

check_image_tags "deploy/k8s/tenant-network/panfs-tenant-network-operator.yaml" "$TENANT_NETWORK_IMAGE_NAME"
check_image_tags "deploy/k8s/storageclass-operator/panfs-storageclass-operator.yaml" "$STORAGECLASS_OPERATOR_IMAGE_NAME"

check_contains "Makefile" "^TENANT_NETWORK_IMAGE[[:space:]]*\?=.*/${TENANT_NETWORK_IMAGE_NAME}:${version_re}\$" \
    "the default TENANT_NETWORK_IMAGE is not '${TENANT_NETWORK_IMAGE_NAME}:${version}'"

check_contains "Makefile" "^STORAGECLASS_OPERATOR_IMAGE[[:space:]]*\?=.*/${STORAGECLASS_OPERATOR_IMAGE_NAME}:${version_re}\$" \
    "the default STORAGECLASS_OPERATOR_IMAGE is not '${STORAGECLASS_OPERATOR_IMAGE_NAME}:${version}'"

check_contains "Makefile" "^CSI_IMAGE[[:space:]]*\?=.*/${IMAGE_NAME}:${version_re}\$" \
    "the default CSI_IMAGE is not '${IMAGE_NAME}:${version}'"

for readme in charts/panfs/Readme.md charts/storageclass/Readme.md charts/tenant-network-operator/Readme.md charts/tenant/Readme.md \
    charts/storageclass-operator/Readme.md; do
    check_contains "$readme" "Version: ${version_re}\]" "no 'Version: ${version}' badge; run 'make manifests'"
    check_contains "$readme" "AppVersion: ${version_re}\]" "no 'AppVersion: ${version}' badge; run 'make manifests'"
done

check_contains "CHANGELOG.md" "^## \[${version_re}\]( |\$)" "no '## [${version}]' entry"
check_contains "README.md" "^\| \*\*${version_re}\*\* " "no row for ${version} in the compatibility matrix"

if ((errors > 0)); then
    printf '\n%d place(s) do not use version %s.\n' "$errors" "$version" >&2
    printf "Set the version there, run 'make manifests', and commit the result.\n" >&2
    exit 1
fi

printf 'OK: charts, manifests, Makefile and documentation use version %s\n' "$version"
