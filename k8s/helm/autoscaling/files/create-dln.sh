#!/bin/bash

set -ex

apt update && apt upgrade -y
apt install -y curl tar gzip git

KUBE_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt | tr -d '[:space:]')
if [ -z "$KUBE_VERSION" ]; then
    echo "ERROR: Failed to retrieve Kubernetes stable version. Exiting setup."
    exit 1
fi
KUBECTL_URL="https://dl.k8s.io/release/$KUBE_VERSION/bin/linux/amd64/kubectl"
curl -LO $KUBECTL_URL
install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

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

########################################################################################
# PEER ORG
########################################################################################
echo -e "\n************* Creating Peer Org **********"
echo -e "\n************* Creating CA for Peer ORG ***********"
kubectl hlf ca create \
    --name ${ORG_CA} \
    --image=${CA_IMAGE} \
    --version=${CA_VERSION} \
    --storage-class=${SC_NAME} \
    --capacity=1Gi \
    --enroll-id=${UENROLL} \
    --enroll-pw=${UENROLLPWD} \
    --hosts=${ORG_CA}.${ORG_DOMAIN} \
    --istio-port=${ISTIO_INGRESS_PORT} \
    -n ${NS}

kubectl wait \
    --timeout=180s \
    --for=condition=Running fabriccas.hlf.kungfusoftware.es \
    -n $NS ${ORG_CA}

sleep 5

echo -e "\n************* Registering Peer User with CA *************"
kubectl hlf ca register \
    --name ${ORG_CA} \
    --user=${UPEER} \
    --secret ${UPEERPWD} \
    --type ${UPEER} \
    --enroll-id ${UENROLL} \
    --enroll-secret ${UENROLLPWD} \
    --mspid ${ORGMSP} \
    -n ${NS} \
    --ca-url="https://${ORG_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"


echo -e "\n************* Creating Initial Peer, peer0 *************"
kubectl hlf peer create \
    --statedb=leveldb \
    --image=${PEER_IMAGE} \
    --version=${PEER_VERSION} \
    --storage-class=${SC_NAME} \
    --name ${PEER_INITIAL} \
    --mspid ${ORGMSP} \
    --enroll-pw=${UPEERPWD} \
    --capacity=${PEER_PVC_SIZE} \
    --enroll-id ${UPEER} \
    --ca-name=${ORG_CA}.${NS} \
    --hosts=${PEER_INITIAL}.${ORG_DOMAIN} \
    --ca-port=${CA_LISTENING_PORT} \
    --istio-port=${ISTIO_INGRESS_PORT} \
    -n ${NS}

kubectl patch fabricpeers "$PEER_INITIAL" -n "$NS" --type merge -p "{
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
    -n $NS ${PEER_INITIAL}

sleep 10

########################################################################################
# ORDERER ORG
########################################################################################
echo -e "\n****** Creating Orderer Org **********"
echo -e "\n****** creating CA for Orderer org *******"
kubectl hlf ca create \
    --name ${ORDERER_CA} \
    --image=${CA_IMAGE} \
    --version=${CA_VERSION} \
    --storage-class=${SC_NAME} \
    --capacity=1Gi \
    --enroll-id=${UENROLL} \
    --enroll-pw=${UENROLLPWD} \
    --hosts=${ORDERER_CA}.${ORG_DOMAIN} \
    --istio-port=${ISTIO_INGRESS_PORT} \
    -n ${NS}

kubectl wait \
    --timeout=180s \
    --for=condition=Running fabriccas.hlf.kungfusoftware.es \
    -n $NS ${ORDERER_CA}

kubectl wait \
    --timeout=180s \
    --for=create deploy/${ORDERER_CA} \
    -n ${NS}

kubectl wait \
    --timeout=180s \
    --for=condition=Available deploy/${ORDERER_CA} \
    -n ${NS} || kubectl wait \
    --timeout=180s \
    --for=condition=Ready pod \
    -l "$(kubectl get deploy ${ORDERER_CA} -n ${NS} -o jsonpath='{range $k,$v := .spec.selector.matchLabels}{printf "%s=%s," $k $v}{end}' | sed 's/,$//')" \
    -n ${NS}

sleep 5

echo -e "\n********* Registering orderer user **********"
orderer_register_max_attempts=12
orderer_register_retry_sleep_seconds=5
orderer_register_attempt=1
orderer_register_success=0

while [ "$orderer_register_attempt" -le "$orderer_register_max_attempts" ]; do
    if kubectl hlf ca register \
        --name ${ORDERER_CA} \
        --user=${UORDERER} \
        --secret ${UORDERERPWD} \
        --type ${UORDERER} \
        --enroll-id ${UENROLL} \
        --enroll-secret ${UENROLLPWD} \
        --mspid ${ORDERER_MSP} \
        -n ${NS} \
        --ca-url="https://${ORDERER_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"; then
        orderer_register_success=1
        break
    fi

    if [ "$orderer_register_attempt" -lt "$orderer_register_max_attempts" ]; then
        echo "Orderer CA registration attempt ${orderer_register_attempt}/${orderer_register_max_attempts} failed; retrying in ${orderer_register_retry_sleep_seconds}s..."
        sleep "$orderer_register_retry_sleep_seconds"
    fi

    orderer_register_attempt=$((orderer_register_attempt + 1))
done

if [ "$orderer_register_success" -ne 1 ]; then
    echo "ERROR: Failed to register orderer user with ${ORDERER_CA} after ${orderer_register_max_attempts} attempts."
    exit 1
fi


echo -e "\n************ Creating Initial Orderer, orderer0 *************"
kubectl hlf ordnode create \
    --image=${ORDERER_IMAGE} \
    --version=${ORDERER_VERSION} \
    --storage-class=${SC_NAME} \
    --enroll-id=${UORDERER} \
    --mspid=${ORDERER_MSP} \
    --enroll-pw=${UORDERERPWD} \
    --capacity=2Gi \
    --name=${ORDERER_INITIAL} \
    --ca-name=${ORDERER_CA}.${NS} \
    --hosts=${ORDERER_INITIAL}.${ORG_DOMAIN} \
    --admin-hosts=${UADMIN}-${ORDERER_INITIAL}.${ORG_DOMAIN} \
    --ca-port=${CA_LISTENING_PORT} \
    --istio-port=${ISTIO_INGRESS_PORT} \
    -n ${NS}

kubectl wait \
    --timeout=180s \
    --for=condition=Running fabricorderernodes.hlf.kungfusoftware.es \
    -n $NS ${ORDERER_INITIAL}

sleep 15

########################################################################################
# CHANNEL CREATION
########################################################################################

################################################
# Register and enrolling OrdererMSP identity
################################################

echo -e "\n********* Creating First Channel *********"
echo -e "\n********* Registering OrdererMSP Identity **********"
kubectl hlf ca register \
    --name=${ORDERER_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --type=${UADMIN} \
    --enroll-id ${UENROLL} \
    --enroll-secret=${UENROLLPWD} \
    --mspid ${ORDERER_MSP} \
    -n ${NS} \
    --ca-url="https://${ORDERER_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"

echo -e "\n************** Enrolling OrdererMSP -> tlsca **************"
kubectl hlf ca enroll \
    --name=${ORDERER_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --mspid ${ORDERER_MSP} \
    --output ${ORDERERMSP_TLS_FILE} \
    --ca-name tlsca \
    --ca-url="https://${ORDERER_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}" \
    -n ${NS}

echo -e "\n******** Enrolling OrdererMSP -> ca *************"
kubectl hlf ca enroll \
    --name=${ORDERER_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --mspid ${ORDERER_MSP} \
    --ca-name ca \
    --output ${ORDERERMSP_FILE} \
    --ca-url="https://${ORDERER_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}" \
    -n ${NS}

echo -e "\n******** Enrolling OrdererMSP -> ca::sign *************"
kubectl hlf ca enroll \
--name=${ORDERER_CA} \
--user=${UADMIN} \
--secret=${UADMINPWD} \
--mspid ${ORDERER_MSP} \
--ca-name ca \
--output ${ORDERERMSP_TLS_SIGN_FILE} \
--ca-url="https://${ORDERER_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}" \
-n ${NS}

################################################
# Register and enrolling Org5MSP Orderer identity
################################################

echo -e "\n********* Registering Org5MSP Orderer Identity **********"
kubectl hlf ca register \
    --name=${ORG_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --type=${UADMIN} \
    --enroll-id ${UENROLL} \
    --enroll-secret=${UENROLLPWD} \
    --mspid=${ORGMSP} \
    -n ${NS} \
    --ca-url="https://${ORG_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"

echo -e "\n************** Enrolling Org5MSP Orderer Identity -> tlsca **************"
kubectl hlf ca enroll \
    --name=${ORG_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --mspid ${ORGMSP} \
    --ca-name tlsca \
    --output ${ORGMSP_TLS_FILE} \
    -n ${NS} \
    --ca-url="https://${ORG_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"

################################################
# Enrolling Org5MSP identity
################################################

echo -e "\n******** Enrolling Org5MSP Indentity -> ca *************"
kubectl hlf ca enroll \
    --name=${ORG_CA} \
    --user=${UADMIN} \
    --secret=${UADMINPWD} \
    --mspid ${ORGMSP} \
    --ca-name ca \
    --output ${ORGMSP_FILE} \
    -n ${NS} \
    --ca-url="https://${ORG_CA}.${ORG_DOMAIN}:${CA_LISTENING_PORT}"

echo -e "\n********** Creating Org5MSP Identity *************"
kubectl hlf identity create \
    --name=${ORGMSP_ID_CR} \
    --ca-name=${ORG_CA} \
    --ca-namespace ${NS} \
    --ca ca \
    --mspid ${ORGMSP} \
    --enroll-id ${UADMIN} \
    --enroll-secret=${UADMINPWD} \
    --namespace ${NS}

##################
# Creating Secret
##################

echo -e "\n************ Creating Channel Secret ************"
kubectl create secret generic wallet \
    --namespace=${NS} \
    --from-file=${ORDERERMSP_FILE}=$PWD/${ORDERERMSP_FILE} \
    --from-file=${ORDERERMSP_TLS_FILE}=$PWD/${ORDERERMSP_TLS_FILE} \
    --from-file=${ORDERERMSP_TLS_SIGN_FILE}=$PWD/${ORDERERMSP_TLS_SIGN_FILE} \
    --from-file=${ORGMSP_FILE}=$PWD/${ORGMSP_FILE} \
    --from-file=${ORGMSP_TLS_FILE}=$PWD/${ORGMSP_TLS_FILE}

echo -e "\n************* Setting Channel Environment Variables ************"
export PEER_ORG_SIGN_CERT=$(kubectl get fabriccas ${ORG_CA} -o=jsonpath='{.status.ca_cert}' -n ${NS})
export PEER_ORG_TLS_CERT=$(kubectl get fabriccas ${ORG_CA} -o=jsonpath='{.status.tlsca_cert}' -n ${NS})
export IDENT_8=$(printf "%8s" "")
export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ${ORDERER_INITIAL} -o=jsonpath='{.status.tlsCert}' -n ${NS} | sed -e "s/^/${IDENT_8}/" )

echo -e "\n*********** Creating Fabric Channel *************"

kubectl delete fabricmainchannel.hlf.kungfusoftware.es ${CHANNEL}-${NS} --ignore-not-found=true
kubectl delete fabricfollowerchannel.hlf.kungfusoftware.es ${CHANNEL}-${NS}-follower --ignore-not-found=true

kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricMainChannel
metadata:
    name: ${CHANNEL}-${NS}
spec:
  name: ${CHANNEL}
  adminOrdererOrganizations:
    - mspID: ${ORDERER_MSP}
  adminPeerOrganizations:
    - mspID: ${ORGMSP}
  channelConfig:
    application:
      acls: null
      capabilities:
        - V2_0
        - V2_5
      policies: null
    capabilities:
      - V2_0
    orderer:
      batchSize:
        absoluteMaxBytes: 49000000
        maxMessageCount: 500
        preferredMaxBytes: ${PREFERRED_MAX_BYTES}
      batchTimeout: 2s
      capabilities:
        - V2_0
      etcdRaft:
        options:
          electionTick: 10
          heartbeatTick: 1
          maxInflightBlocks: 5
          snapshotIntervalSize: 16777216
          tickInterval: 500ms
      ordererType: etcdraft
      policies: null
      state: STATE_NORMAL
    policies: null
  externalOrdererOrganizations: []
  externalPeerOrganizations: []
  peerOrganizations:
    - mspID: ${ORGMSP}
      caName: "${ORG_CA}"
      caNamespace: "${NS}"
  identities:
    ${ORDERER_MSP}:
      secretKey: ${ORDERERMSP_FILE}
      secretName: wallet
      secretNamespace: ${NS}
    ${ORDERER_MSP}-tls:
      secretKey: ${ORDERERMSP_TLS_FILE}
      secretName: wallet
      secretNamespace: ${NS}
    ${ORDERER_MSP}-sign:
      secretKey: ${ORDERERMSP_TLS_SIGN_FILE}
      secretName: wallet
      secretNamespace: ${NS}
    ${ORGMSP}:
      secretKey: ${ORGMSP_FILE}
      secretName: wallet
      secretNamespace: ${NS}
    ${ORGMSP}-tlsca:
      secretKey: ${ORGMSP_TLS_FILE}
      secretName: wallet
      secretNamespace: ${NS}

  ordererOrganizations:
    - caName: "${ORDERER_CA}"
      caNamespace: "${NS}"
      externalOrderersToJoin:
        - host: ${ORDERER_INITIAL}.${ORG_DOMAIN}
          port: ${CHANNEL_JOIN_PORT}
      mspID: ${ORDERER_MSP}
      ordererEndpoints:
        - ${ORDERER_INITIAL}.${ORG_DOMAIN}:${ORDERER_TLS_PORT}
      orderersToJoin: []
  orderers:
    - host: ${ORDERER_INITIAL}.${ORG_DOMAIN}
      port: ${ORDERER_TLS_PORT}
      tlsCert: |-
${ORDERER0_TLS_CERT}
EOF

kubectl wait \
    --timeout=600s \
    --for=condition=Running fabricmainchannel.hlf.kungfusoftware.es \
    -n $NS ${CHANNEL}-${NS}
sleep 5

########################################################################################
# Peer Join Channel
########################################################################################

echo -e "\e************** Joining Peer0 to Channel ****************"
kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricFollowerChannel
metadata:
  name: ${CHANNEL}-${NS}-follower
spec:
  anchorPeers:
    - host: ${PEER_INITIAL}.${ORG_DOMAIN}
      port: ${PEER_JOIN_PORT}
  hlfIdentity:
    secretKey: ${ORGMSP_FILE}
    secretName: wallet
    secretNamespace: ${NS}
  mspId: ${ORGMSP}
  name: ${CHANNEL}
  externalPeersToJoin: []
  orderers:
    - certificate: |
${ORDERER0_TLS_CERT}
      url: grpcs://${ORDERER_INITIAL}.${ORG_DOMAIN}:${ORDERER_TLS_PORT}
  peersToJoin:
    - name: ${PEER_INITIAL}
      namespace: ${NS}
EOF

kubectl wait \
    --timeout=600s \
    --for=condition=Running fabricfollowerchannel.hlf.kungfusoftware.es \
    -n $NS ${CHANNEL}-${NS}-follower
sleep 5

########################################################################################
# Chaincode
########################################################################################

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
cat << METADATA-EOF > "metadata.json"
{
    "type": "ccaas",
    "label": "${CHAINCODE_LABEL}"
}
METADATA-EOF

echo -e "\n************* Creating Connection File ************"
cat << CONN_EOF > "connection.json"
{
    "address": "${CHAINCODE_NAME}:7052",
    "dial_timeout": "10s",
    "tls_required": false
}
CONN_EOF


echo -e "\n************ Building Chaincode archives *************"
tar cfz code.tar.gz connection.json
tar cfz chaincode.tgz metadata.json code.tar.gz

export PACKAGE_ID=$(kubectl hlf chaincode calculatepackageid --path=chaincode.tgz --language=node --label=$CHAINCODE_LABEL)
echo "PACKAGE_ID=${PACKAGE_ID}"

echo -e "\n**************** Installing Chaincode **************"
kubectl hlf chaincode install \
    --path=./chaincode.tgz \
    --config=${ORG_INSPECT_FILE} \
    --language=golang \
    --label=$CHAINCODE_LABEL \
    --user=${UADMIN} \
    --peer=${PEER_INITIAL}.${NS}

# Avoid race condition
echo -e "\n******** Sleeping 30 seconds to avoid race condition ********"
sleep 30

echo -e "\n**************** Deploying Corsha Chaincode **************"
kubectl hlf externalchaincode sync \
    --image=${CHAINCODE_IMAGE} \
    --name=${CHAINCODE_NAME} \
    --namespace=${NS} \
    --package-id=${PACKAGE_ID} \
    --tls-required=false \
    --replicas=1

kubectl wait \
    --timeout=360s \
    --for=condition=Running fabricchaincodes.hlf.kungfusoftware.es \
    -n ${NS} ${CHAINCODE_NAME}
sleep 60

echo -e "\n****************** Approve, Comit, and Invoke transaction on Deployed Chaincode *****************"
echo -e "\n****************** Approve *****************"
kubectl hlf chaincode approveformyorg \
    --config=${ORG_INSPECT_FILE} \
    --user=${UADMIN} \
    --peer=${PEER_INITIAL}.${NS} \
    --package-id=${PACKAGE_ID} \
    --version "${CHAINCODE_APPROVAL_VERSION}" \
    --sequence "${CHAINCODE_SEQUENCE}" \
    --name=${CHAINCODE_NAME} \
    --policy="OR('${ORGMSP}.member')" --channel=${CHANNEL}
sleep 10

echo -e "\n********** Commit **************"
kubectl hlf chaincode commit \
  --config=${ORG_INSPECT_FILE} \
  --user=${UADMIN} \
  --mspid=${ORGMSP} \
  --version "${CHAINCODE_APPROVAL_VERSION}" \
  --sequence "${CHAINCODE_SEQUENCE}" \
  --name=${CHAINCODE_NAME} \
  --policy="OR('${ORGMSP}.member')" \
  --channel=${CHANNEL}
sleep 10

echo -e "\n*************** Invoke **************"
kubectl hlf chaincode invoke \
    --config=${ORG_INSPECT_FILE} \
    --user=${UADMIN} \
    --peer=${PEER_INITIAL}.${NS} \
    --chaincode=${CHAINCODE_NAME} \
    --channel=${CHANNEL} \
    --fcn=pingChaincode

# upload the config to the secret
kubectl create secret generic dln-config --from-file=config=${ORG_INSPECT_FILE} -n ${NS}
