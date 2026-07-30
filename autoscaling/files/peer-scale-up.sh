set -e

apt update && apt upgrade
apt install -y curl tar gzip git yq
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

COUNT=$(kubectl -n $NS get pods -l app=hlf-peer --no-headers 2>/dev/null | wc -l)
export PEER="peer$((COUNT+1))-${ORGL}"

########################################################################################
# create New Peer
########################################################################################

echo -e "\n************* Creating New Peer *************"
kubectl hlf peer create \
    --statedb=leveldb \
    --image=${PEER_IMAGE} \
    --version=${PEER_VERSION} \
    --storage-class=${SC_NAME} \
    --name ${PEER} \
    --mspid ${ORGMSP} \
    --enroll-pw=${UPEERPWD} \
    --capacity=${PEER_PVC_SIZE} \
    --enroll-id ${UPEER} \
    --ca-name=${ORG_CA}.${NS} \
    --hosts=${PEER}.${ORG_DOMAIN} \
    --ca-port=${CA_LISTENING_PORT} \
    --istio-port=${ISTIO_INGRESS_PORT} \
    -n ${NS}

kubectl patch fabricpeers "$PEER" -n "$NS" --type merge -p "{
  \"spec\": {
    \"resources\": {
      \"peer\": {
        \"limits\": {
          \"cpu\": \"${PEER_CPU}\",
          \"memory\": \"${PEER_RAM}\"
        },
        \"requests\": {
          \"cpu\": \"${PEER_CPU}\",
          \"memory\": \"${PEER_RAM}\"
        }
      }
    }
  }
}"

kubectl wait \
    --timeout=180s \
    --for=condition=Running fabricpeers.hlf.kungfusoftware.es \
    -n ${NS} ${PEER}

sleep 10

########################################################################################
# New Peer Join Channel
########################################################################################

export IDENT_8=$(printf "%8s" "")
export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ${ORDERER_INITIAL} -o=jsonpath='{.status.tlsCert}' -n ${NS} | sed -e "s/^/${IDENT_8}/" )

kubectl get fabricfollowerchannel ${CHANNEL}-${NS}-follower -n ${NS} -o yaml > tmp.yaml
/usr/local/bin/yq_go -i '.spec.peersToJoin += [{"name": env(PEER), "namespace": env(NS)}]' tmp.yaml
kubectl apply -f tmp.yaml
sleep 15

########################################################################################
# Chaincode
########################################################################################

kubectl wait \
    --timeout=600s \
    --for=jsonpath='{.status.status}'=RUNNING fabricfollowerchannel.hlf.kungfusoftware.es \
    -n $NS ${CHANNEL}-${NS}-follower
sleep 5

echo -e "\n************** Deploying Chaincode ********************"
echo -e "\n************** Getting Connection String **************"
kubectl hlf inspect \
    --output ${ORG_INSPECT_FILE} \
    -o ${ORGMSP} \
    -o ${ORDERER_MSP} \
    -n ${NS}

echo -e "\n**************** Enrolling Admin User ****************"
kubectl hlf ca enroll \
    --name=${ORG_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --mspid ${ORGMSP} \
    --ca-name ca \
    --output ${PEER_CHAINCODE_ENROLL_FILE} \
    --ca-url="https://${ORG_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}" \
    -n ${NS}

echo -e "\n***************** Attaching User to Connection String ****************"
kubectl hlf utils adduser \
    --userPath=${PEER_CHAINCODE_ENROLL_FILE} \
    --config=${ORG_INSPECT_FILE} \
    --username=${UADMIN} \
    --mspid=${ORGMSP}

echo -e "\n************** Creating Metadata *************"
printf '{ "type": "ccaas", "label": "%s" }\n' "$CHAINCODE_LABEL" > metadata.json

printf '{ "address": "%s:7052", "dial_timeout": "10s", "tls_required": false }\n' \
  "$CHAINCODE_NAME" > connection.json

echo -e "\n************ Building Chaincode archives *************"
tar cfz code.tar.gz connection.json
tar cfz chaincode.tgz metadata.json code.tar.gz

echo -e "\n**************** Installing Chaincode **************"
kubectl hlf chaincode install \
    --path=./chaincode.tgz \
    --config=${ORG_INSPECT_FILE} \
    --language=golang \
    --label=$CHAINCODE_LABEL \
    --user=${UADMIN} \
    --peer=${PEER}.${NS}

echo -e "\n**************** Deploying Corsha Chaincode **************"
REPLICAS=$((COUNT + 1))
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

echo -e "\n*************** Invoke **************"
kubectl hlf chaincode invoke \
    --config=${ORG_INSPECT_FILE} \
    --user=${UADMIN} \
    --peer=${PEER}.${NS} \
    --chaincode=${CHAINCODE_NAME} \
    --channel=${CHANNEL} \
    --fcn=pingChaincode

# Upload the updated config
kubectl create secret generic dln-config \
  --from-file=config="${ORG_INSPECT_FILE}" \
  -n "${NS}" \
  --dry-run=client -o yaml | kubectl apply -f -
