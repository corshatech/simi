set -ex

# --- INSTALLATION PRE-REQS ---
apt update && apt upgrade -y
apt install -y curl tar gzip git
apt install -y kubectl

(
    set -x; cd "$(mktemp -d)" &&
    OS="$(uname | tr '[:upper:]' '[:lower:]')" &&
    ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" &&
    KREW="krew-${OS}_${ARCH}" &&
    curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/${KREW}.tar.gz" &&
    tar zxvf "${KREW}.tar.gz" &&
    ./"${KREW}" install krew
)

export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"

kubectl krew install hlf

YQ_VERSION=$(curl -s https://api.github.com/repos/mikefarah/yq/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
YQ_URL="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64"

curl -L "${YQ_URL}" -o /usr/local/bin/yq_go
chmod +x /usr/local/bin/yq_go

# --- IDENTIFY RESOURCES ---
# We identify the peer to remove, but we DO NOT delete it yet.
NEWEST_PEER=$(kubectl -n ${NS} get fabricpeers --sort-by=.metadata.creationTimestamp -o=name | tac | head -1)
export PEER=$(echo $NEWEST_PEER | awk -F'/' '{print $2}')

# Guard: Prevent scaling down the initial peer (peer0)
if [[ "$PEER" == "peer0"* ]]; then
    echo "Refusing to scale down initial peer: $PEER. Autoscaling limit reached."
    exit 0
fi

export PEER_FQDN="${PEER}.${NS}"

# --- STEP 1: UPDATE CONFIGURATION FIRST ---
# We update the secret immediately so clients stop trying to connect to this peer
# while the infrastructure is still shutting down.

echo -e "\n***************** Remove Peer from Config ****************"
kubectl get secret dln-config -n $NS -o jsonpath='{.data.config}' | base64 --decode > ${ORG_INSPECT_FILE}

# Remove the peer from the config file locally
/usr/local/bin/yq_go -i 'del(.peers[env(PEER_FQDN)]) | del(.channels._default.peers[env(PEER_FQDN)]) | .organizations.Org1MSP.peers |= map(select(. != env(PEER_FQDN)))' "${ORG_INSPECT_FILE}"

# Upload the updated config immediately
kubectl create secret generic dln-config \
  --from-file=config="${ORG_INSPECT_FILE}" \
  -n "${NS}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Config updated. Clients should now stop routing traffic to ${PEER_FQDN}."

# --- STEP 2: PREPARE SCALING MATH ---
# Calculate target replicas BEFORE deleting the peer to avoid race conditions
# in counting pods that are in 'Terminating' state.
CURRENT_COUNT=$(kubectl -n $NS get pods -l app=hlf-peer --no-headers 2>/dev/null | wc -l)
REPLICAS=$((CURRENT_COUNT - 1))

# --- STEP 3: SCALE DOWN INFRASTRUCTURE ---

# 3a. Remove from FabricFollowerChannel
kubectl get fabricfollowerchannel ${CHANNEL}-${NS}-follower -n ${NS} -o yaml > tmp.yaml
/usr/local/bin/yq_go -i 'del(.spec.peersToJoin[] | select(.name == env(PEER) and .namespace == env(NS)))' tmp.yaml
kubectl apply -f tmp.yaml

# 3b. Delete the FabricPeer (The actual infrastructure scaling)
echo "Deleting Peer ${NEWEST_PEER}..."
kubectl -n ${NS} delete ${NEWEST_PEER}

# 3c. Update Chaincode Replicas
echo "Scaling chaincode to ${REPLICAS} replicas..."
kubectl patch fabricchaincode ${CHAINCODE_NAME} -n ${NS} --type=merge -p "{
  \"spec\": {
    \"replicas\": ${REPLICAS}
  }
}"

# --- STEP 4: WAIT FOR CONVERGENCE ---

kubectl wait \
    --timeout=600s \
    --for=jsonpath='{.status.status}'=RUNNING fabricfollowerchannel.hlf.kungfusoftware.es \
    -n $NS ${CHANNEL}-${NS}-follower

kubectl -n ${NS} rollout status deployment ${CHAINCODE_NAME} --timeout=300s

kubectl wait \
    --timeout=360s \
    --for=condition=Running fabricchaincodes.hlf.kungfusoftware.es \
    -n ${NS} ${CHAINCODE_NAME}

sleep 10
echo "Scale down complete."
