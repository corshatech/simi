#!/bin/bash

# Copyright Corsha Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

BENCH_DIR="$(
  cd "$(dirname "$0")"
  pwd -P
)"

export PREFIX=""
if [ -n "$SIMI_JOB_NAME" ]; then
  PREFIX="${SIMI_JOB_NAME}-"
fi

if [ -z "$TARGET_NS" ]; then
  echo -e "TARGET_NS var not defined. what is the name of the namespace with the DLN you would like to test?"
  read -r target
  export TARGET_NS="$target"
fi

# check for SIMI_NAMESPACE, prompt if missing
if [ -z "$SIMI_NAMESPACE" ]; then
  export SIMI_NAMESPACE="${TARGET_NS}-benchmark"
fi
shopt -s nocasematch
if [[ "${TARGET_NS}" == "${SIMI_NAMESPACE}" ]]; then
  echo "ERROR - YOU CANNOT RUN THE SIMI IN THE SAME NAMESPACE AS WHAT IT IS TESTING. use a different SIMI_NAMESPACE value"
  exit 1
fi
shopt -u nocasematch
echo "your simi will run in the namespace: $SIMI_NAMESPACE"

if [[ $(kubectl get ns | grep -wc "^${SIMI_NAMESPACE} ") -gt 0 ]]; then
  echo -e "namespace $SIMI_NAMESPACE already exists. would you like to delete it before running?[Y/(n)]"
  read -r clean

  if [[ $clean =~ ^[Yy]$ ]]; then
    echo "deleting $SIMI_NAMESPACE"
    helm ls -q -a --namespace="$SIMI_NAMESPACE" | xargs -I {} helm uninstall {} --namespace="$SIMI_NAMESPACE"
    kubectl delete ns "$SIMI_NAMESPACE" --force --grace-period=0

    while [[ $(kubectl get ns | grep -Ec "$SIMI_NAMESPACE") -gt 0 ]]; do
      echo -e "waiting on ns deletion..."
      sleep 10
    done

  else
    echo "did not delete existing deployment"
  fi

fi

if [ -z "$SIMI_OPERATION_TYPE" ]; then
  echo -e "SIMI_OPERATION_TYPE var not defined. What would you like to call this chaincode operation? "
  read -r operation_type
  export SIMI_OPERATION_TYPE="$operation_type"
fi

if [ -z "$SIMI_OPERATION_PERIOD" ]; then
  echo -e "SIMI_OPERATION_PERIOD var not defined. how long would you like each client to wait between operations (e.g. 5s, 1m)?"
  read -r period
  export SIMI_OPERATION_PERIOD="$period"
fi

if [ -z "$SIMI_NUM_WORKERS" ]; then
  echo -e "SIMI_NUM_WORKERS var not defined. how many workers would you like to spawn?"
  read -r workers
  export SIMI_NUM_WORKERS="$worker"
fi

export SIMI_STREAMS_PER_DEVICE=1

if [ -z "$SIMI_OPERATIONS_PER_STREAM" ]; then
  echo -e "SIMI_OPERATIONS_PER_STREAM var not defined. how many times should each stream be written to?"
  read -r ops
  export SIMI_OPERATIONS_PER_STREAM="$ops"
fi

EPOCH_TIME=$(date +%s)
name=$(echo "${PREFIX}${SIMI_NAMESPACE}-${SIMI_OPERATION_TYPE}-p${SIMI_OPERATION_PERIOD}-d${SIMI_NUM_WORKERS}-w${SIMI_OPERATIONS_PER_STREAM}-${EPOCH_TIME}" | tr '[:upper:]' '[:lower:]')

echo "Job name is [$name]"

if [ -n "$SIMI_HELM_CHART" ]; then
  pushd "$SIMI_HELM_CHART"
  helm dependency update
  popd
fi

# We define an ingress hostname for use by the curl script below
# This can be removed if the results are retrieved differently e.g. `kubectl cp`
INFLUX_HOST="influx-$SIMI_NAMESPACE.kaspean.biz"

# helm install
helmfile -f "$BENCH_DIR""/../k8s/helmfiles/benchmarking/simi/helmfile.yaml" apply --set="influxdb.ingress.hostname=$INFLUX_HOST,shouldFailOnError=true,uniqueId=$name,numWorkers=$SIMI_NUM_WORKERS,operationPeriod=$SIMI_OPERATION_PERIOD,operationsPerStream=$SIMI_OPERATIONS_PER_STREAM,operationType=$SIMI_OPERATION_TYPE,targetNamespace=$TARGET_NS"

kubectl label ns "$SIMI_NAMESPACE" com.corsha.ecm.disruptible=false --overwrite
sleep 5

get_statuses() {
  simi_status=$(kubectl -n "$SIMI_NAMESPACE" get jobs -o go-template --template='{{range .items}}{{if and ( eq .metadata.name "simi") ( .status.conditions) }}{{(index .status.conditions 0).type }}{{end}}{{ end }}')
  consumer_status=$(kubectl -n "$SIMI_NAMESPACE" get jobs -o go-template --template='{{range .items}}{{if and (eq .metadata.name "consumer") ( .status.conditions) }}{{(index .status.conditions 0).type }}{{end}}{{ end }}')
  echo -e "status:\n  consumer: $consumer_status\n  simi: $simi_status\n"
}

get_statuses

while [[ $(get_statuses | grep -Ec "Failed|Complete") -lt 1 ]]; do
  echo -e "waiting on simi.\n"
  get_statuses

  echo "pods:"

  kubectl -n "$SIMI_NAMESPACE" get pods

  if [[ $(kubectl -n "$SIMI_NAMESPACE" get pods | grep -Ec "Terminating|Failed") -gt 0 ]]; then
    echo "job failed because a pod failed"
    exit 1
  fi

  sleep 10
done

get_statuses

if [[ $(get_statuses | grep -Ec "Failed") -ge 1 ]]; then
  echo "job failed. inspect container logs"
  exit 1
fi

mkdir -p "out/${name}"

INFLUX_TOKEN=$(kubectl -n "$SIMI_NAMESPACE" get secrets influx-simi -o yaml | yq '.data["admin-user-token"]' | base64 -d)

curl --request POST \
  --retry 20 \
  --retry-connrefused \
  --output "./out/${name}/perf.csv.gz" \
  "https://$INFLUX_HOST/api/v2/query?org=simi" \
  --header "Authorization: Token $INFLUX_TOKEN" \
  --header 'Accept: application/csv' \
  --header 'Content-type: application/vnd.flux' \
  --header 'Accept-Encoding: gzip' \
  --data 'from(bucket: "stats")
  |> range(start: -1mo) // Assuming this influx instance only holds results from this simi run
  |> drop(columns: ["_start", "_stop", "_field"])'
echo "results saved to ./out/${name}/perf.csv.gz"

# Write out current name of the job to out/benchmark.json so that tools can know what the name of the current job is
jq -nc \
  --arg timestamp            "$EPOCH_TIME" \
  --arg namespace            "$TARGET_NAMESPACE" \
  --arg numWorkers           "$SIMI_NUM_WORKERS" \
  --arg operationPeriod      "$SIMI_OPERATION_PERIOD" \
  --arg operationType        "$SIMI_OPERATION_TYPE" \
  --arg operationsPerStream  "$SIMI_OPERATIONS_PER_STREAM" \
  --arg outDir               "out/${name}" \
  '$ARGS.named' > out/benchmark.json

cp out/benchmark.json "./out/${name}/"
echo "stopping the job..."

