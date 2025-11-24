#!/bin/bash
# Create a safe temporary directory
#!/bin/bash
# Create a safe temporary directory
TMPDIR=$(mktemp -d)
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT

export GREEN="\033[32m"
export RESET="\033[0m"
export YELLOW="\033[33m"
export RED="\033[31m"
export CYAN="\033[36m"
export GRAY="\033[90m"
export BOLD="\033[1m"

print() {
    printf "%b\n" "$1"
}

print_list() {
    if [ -z "${1}${2}" ]; then
        print "      No resources found in ${NS} namespace."
        return
    fi

    printf "%s\n" "$1" | tr ',' '\n' > "$TMPDIR/names"
    printf "%s\n" "$2" | tr ',' '\n' > "$TMPDIR/images"
    paste "$TMPDIR/names" "$TMPDIR/images" | awk '{ printf "      %-30s %s\n", $1":", $2 }'
    rm -f "$TMPDIR/names" "$TMPDIR/images"
}

errors=0
NS="csi-panfs"

#############################
# 1. Controller             #
#############################
print "${BOLD}CSI/Controller health...${RESET}"
print "\n  Checks:\n"

if ! kubectl get deploy -n "${NS}" csi-panfs-controller >/dev/null 2>&1; then
    print "    ${YELLOW}⚠ Controller deployment not found. Skipping controller checks.${RESET}"
else
    csi_controller_deployed="true"
    controller_ready=$(kubectl get deploy -n "${NS}" csi-panfs-controller -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
    controller_expected=$(kubectl get deploy -n "${NS}" csi-panfs-controller -o jsonpath='{.status.replicas}' 2>/dev/null || echo "0")
    if [ "$controller_ready" = "$controller_expected" ] && [ "$controller_ready" != "0" ]; then
        print "    ${GREEN}✔ Controller deployment is healthy ($controller_ready/$controller_expected Ready)${RESET}"
    else
        print "    ${RED}✘ Controller deployment is NOT healthy ($controller_ready/$controller_expected Ready)${RESET}"
        errors=$((errors+1))
    fi

    bad_controller_pods=$(kubectl get pods -n "${NS}" -l app=csi-panfs-controller --no-headers 2>/dev/null | awk '{split($2, a, "/");if (a[1] != a[2] || $3 != "Running")print}')
    if [ -z "$bad_controller_pods" ] && [ "$controller_ready" != "0" ]; then
        print "    ${GREEN}✔ All controller pods Running & Ready${RESET}"
    else
        if [ "$controller_ready" == "0" ]; then
            print "    ${RED}✘ No controller pods are running${RESET}"
        else
            print "    ${RED}✘ Some controller pods are not ready:${RESET}"
        fi
        errors=$((errors+1))
    fi

    if [ -n "${CSI_IMAGE}" ] && [ "$controller_ready" != "0" ] ; then
        csi_image=$(kubectl get deploy -n "${NS}" csi-panfs-controller -o jsonpath='{.spec.template.spec.containers[?(@.name=="csi-panfs-plugin")].image}' 2>/dev/null || print "")
        if [ "${csi_image}" != "${CSI_IMAGE}" ]; then
            print "    ${RED}✘ CSI image mismatch. Expected: ${CSI_IMAGE}, Found: ${csi_image}${RESET}"
            errors=$((errors+1))
        else
            print "    ${GREEN}✔ CSI image matches expected: ${CSI_IMAGE}${RESET}"
        fi
    else 
        if [ "$controller_ready" != "0" ]; then
            print "    ${CYAN}⚠ Please set CSI_IMAGE environment variable to validate the CSI image.${RESET}"
        fi
    fi

    print "\n  Details:"
    print "\n    Deployment status:"
    kubectl get deploy -n "${NS}" 2>&1 | sed 's/^/      /'

    print "\n    Pods status:"
    kubectl get pods -n "${NS}" -l app=csi-panfs-controller 2>&1 | sed 's/^/      /'

    print "\n    Containers and Images:"
    names="$(kubectl get deploy -n "${NS}" -o wide --no-headers | awk '{print $6}')"
    images="$(kubectl get deploy -n "${NS}" -o wide --no-headers | awk '{print $7}')"
    print_list "$names" "$images"
fi

###########################
# 2. Node                 #
###########################
dfc_image=""
if [ -n "${DFC_REGISTRY}" ] && [ -n "${DFC_VERSION}" ]; then
    dfc_image="${DFC_REGISTRY}/panfs-dfc:${DFC_VERSION}"
fi

print "\n${BOLD}CSI/Node health...${RESET}"
print "\n  Checks:\n"
if ! kubectl get ds -n "${NS}" csi-panfs-node >/dev/null 2>&1; then
    print "    ${YELLOW}⚠ Node daemonset not found. Skipping node checks.${RESET}"
else
    csi_node_deployed="true"
    ds_ready=$(kubectl get ds -n "${NS}" csi-panfs-node -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")
    ds_expected=$(kubectl get ds -n "${NS}" csi-panfs-node -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")

    if [ "$ds_ready" = "$ds_expected" ] && [ "$ds_ready" != "0" ]; then
        print "    ${GREEN}✔ Node DaemonSet is healthy ($ds_ready/$ds_expected Ready)${RESET}"
    else
        print "    ${RED}✘ Node DaemonSet is NOT healthy ($ds_ready/$ds_expected Ready)${RESET}"
        errors=$((errors+1))
    fi

    bad_node_pods=$(kubectl get pods -n "${NS}" -l app=csi-panfs-node --no-headers 2>/dev/null | awk '{split($2, a, "/");if (a[1] != a[2] || $3 != "Running")print}')
    if [ -z "$bad_node_pods" ] && [ "$ds_ready" != "0" ]; then
        print "    ${GREEN}✔ All node pods Running & Ready${RESET}"
    else
        if [ "$ds_ready" == "0" ]; then
            print "    ${RED}✘ No node pods are running${RESET}"
        else
            print "    ${RED}✘ Some node pods are not ready:${RESET}"
        fi
        errors=$((errors+1))
    fi

    if [ -n "${dfc_image}" ] && [ "$ds_ready" != "0" ]; then
        node_image=$(kubectl get ds -n "${NS}" csi-panfs-node -o jsonpath='{.spec.template.spec.containers[?(@.name=="csi-panfs-plugin")].image}' 2>/dev/null || print "")
        if [ "${node_image}" != "${dfc_image}" ]; then
            print "    ${RED}✘ DFC image mismatch. Expected: ${dfc_image}, Found: ${node_image}${RESET}"
            errors=$((errors+1))
        else
            print "    ${GREEN}✔ DFC image matches expected: ${dfc_image}${RESET}"
        fi
    else
        if [ "$ds_ready" != "0" ]; then
            print "    ${CYAN}⚠ Please set DFC_REGISTRY and DFC_VERSION environment variables to validate the DFC image.${RESET}"
        fi
    fi

    print "\n  Details:"
    print "\n    DaemonSet status:"
    kubectl get ds -n "${NS}" 2>&1 | sed 's/^/      /'

    print "\n    Pods status:"
    kubectl get pods -n "${NS}" -l app=csi-panfs-node 2>&1 | sed 's/^/      /'

    print "\n    Containers and Images:"
    names=$(kubectl get ds -n "${NS}" -o wide --no-headers | awk '{print $9}')
    images=$(kubectl get ds -n "${NS}" -o wide --no-headers | awk '{print $10}')
    print_list "$names" "$images"
fi

###########################
# 3. Kernel Module Status #
###########################
print "\n${BOLD}Kernel Module status...${RESET}"
print "\n  Checks:"
if ! kubectl get module -n "${NS}" panfs >/dev/null 2>&1; then
    print "    ${YELLOW}⚠ Kernel module CR not found — continuing (only needed if kernel module mode is enabled)${RESET}"
else
    module_ready=$(kubectl get module -n "${NS}" panfs -o jsonpath='{.status.moduleLoader.availableNumber}' 2>/dev/null || echo "0")
    module_desired=$(kubectl get module -n "${NS}" panfs -o jsonpath='{.status.moduleLoader.desiredNumber}' 2>/dev/null || echo "0")
    if [ "$module_ready" = "$module_desired" ] && [ "$module_ready" != "0" ]; then
        print "    ${GREEN}✔ Kernel module is healthy ($module_ready/$module_desired Loaded)${RESET}"
    else
        print "    ${RED}✘ Kernel module is NOT healthy ($module_ready/$module_desired Loaded)${RESET}"
        errors=$((errors+1))
    fi

    print "\n  Details:"
    kubectl describe module -n "${NS}" panfs | sed -n '/^Status/,/^$/p' | sed 's/^/    /'
fi

###########################
# Final verdict           #
###########################
print "\n${BOLD}Validation Status:${RESET}"
if [ "$errors" -eq 0 ]; then
    if [ -n "${csi_controller_deployed}" ] && [ -n "${csi_node_deployed}" ]; then
        print "${GREEN}✔ All checks passed. Safe to deploy the StorageClass.${RESET}"
        exit 0
    else
        if [ -n "${csi_controller_deployed}" ] || [ -n "${csi_node_deployed}" ]; then
            print "${YELLOW}⚠ Partial CSI components detected in the cluster. Please ensure that both the PanFS CSI Controller and Node components are deployed in the ${NS} namespace.${RESET}"
            exit 1
        else
            print "${YELLOW}⚠ No CSI components detected in the cluster.${RESET}"
            exit 0
        fi
    fi
else
    print "${RED}✘ $errors issue(s) detected.${RESET}"
    exit 1
fi