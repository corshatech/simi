set -ex

apt update && apt upgrade
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

NEWEST_PEER=$(kubectl -n ${NS} get fabricpeers --sort-by=.metadata.creationTimestamp -o=name | tac | head -1)
export PEER=$(echo $NEWEST_PEER | awk -F'/' '{print $2}')

kubectl get fabricfollowerchannel ${CHANNEL}-${NS}-follower -n ${NS} -o yaml > tmp.yaml
/usr/local/bin/yq_go -i 'del(.spec.peersToJoin[] | select(.name == env(PEER) and .namespace == env(NS)))' tmp.yaml
kubectl apply -f tmp.yaml

# delete fabricpeer
kubectl -n ${NS} delete ${NEWEST_PEER}

kubectl wait \
    --timeout=600s \
    --for=jsonpath='{.status.status}'=RUNNING fabricfollowerchannel.hlf.kungfusoftware.es \
    -n $NS ${CHANNEL}-${NS}-follower

echo -e "\n***************** Remove Peer from Config ****************"
kubectl get secret dln-config -n $NS -o jsonpath='{.data.config}' | base64 --decode > ${ORG_INSPECT_FILE}

export PEER_FQDN="${PEER}.${NS}"
/usr/local/bin/yq_go -i 'del(.peers[env(PEER_FQDN)]) | del(.channels._default.peers[env(PEER_FQDN)]) | .organizations.Org1MSP.peers |= map(select(. != env(PEER_FQDN)))' "${ORG_INSPECT_FILE}"

COUNT=$(kubectl -n $NS get pods -l app=hlf-peer --no-headers 2>/dev/null | wc -l)
REPLICAS=$((COUNT - 1))
kubectl patch fabricchaincode ${CHAINCODE_NAME} -n ${NS} --type=merge -p "{
  \"spec\": {
    \"replicas\": ${REPLICAS}
  }
}"
kubectl -n ${NS} rollout status deployment ${CHAINCODE_NAME} --timeout=300s

kubectl wait \
    --timeout=360s \
    --for=condition=Running fabricchaincodes.hlf.kungfusoftware.es \
    -n ${NS} ${CHAINCODE_NAME}
sleep 10

# Upload the updated config
kubectl create secret generic dln-config \
  --from-file=config="${ORG_INSPECT_FILE}" \
  -n "${NS}" \
  --dry-run=client -o yaml | kubectl apply -f -
