# Simi

Simi is a tool that can be used to rapidly populate a [Hyperledger Fabric](https://www.lfdecentralizedtrust.org/projects/fabric) DLN ledger with a large amount of data. It does so by running a Kubernetes job that deploys a
number of Simi worker pods who each run a chaincode operation a number of times at a time interval.

Simi tracks duration each chaincode operation takes to complete and records it as a data point in Influx DB. Upon completion of the Simi job, it will output its results to a directory in `./out/`. 

## Running Simi with a Custom Chaincode Operation

### Custom Chaincode Go Code

By default, Simi is configured to perform a `pingChaincode` operation as an example. For more information about the `pingChaincode` example and how to deploy it, see the [ping README](./ping/README.md). 

To run Simi with a custom chaincode operation, you will need to implement an instance of the `WorkerConfig` type defined here: [./config/config.go](./config/config.go) for your chaincode operation. The `WorkerConfig` type is used by Simi to operate a Simi worker.

Once your instance has been implemented, build your changes into a Simi image with the following commands:
```bash
$ make simi
$ make simi-image
```

### Helm Chart Custom Configuration
The helm chart is configured to expect custom configuration for your `OperationConfig` in the `.Values.operationConfig` object and will template that object into the Simi job's mounted configuration file, `simi.yaml`, when deploying. 

In order to run against your DLN, replace the contents of [./k8s/helm/simi/fabric-config.yaml](./k8s/helm/simi/fabric-config.yaml) with the fabric-sdk config.yaml file for your DLN. Then either package the chart or set `SIMI_HELM_CHART` environment variable to point to your local Chart directory. 

For more information on the Simi Helm chart, see the Helm chart [README](./k8s/helm/simi/README.md).

# Deploying Simi

> **_NOTE:_**  The Simi project does not publish images or Helm charts. You must build and publish your own.

Simi can be deployed by running the [./benchmark/launch.sh](./benchmark/launch.sh) script. The script will prompt the user for the target namespace, the number of workers, the number of chaincode operations per worker, and the period between operation. To bypass the script's prompting, set the following env vars.

```bash
export TARGET_NS=simi-test
export SIMI_OPERATION_PERIOD=1s
export SIMI_NUM_WORKERS=500
export SIMI_OPERATIONS_PER_STREAM=100
export SIMI_OPERATION_TYPE=ping
```

This example will setup 500 Simi workers and perform 100 `ping` Chaincode operations 1s apart on each of them.

When the benchmark finishes, it will output its results to a directory in `./out/`
