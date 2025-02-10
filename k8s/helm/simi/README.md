# The Simi Helm Chart

## Deploying the Simi Helm Chart

In order to use Simi with your DLN, you will need to replace the [fabric-config.yaml](./fabric-config.yaml) with the fabric-config for your Hyperledger Fabric DLN. Once it is replaced, the helm chart can either be packaged and published, or deployed from the local directory. 

To deploy from the local chart directory with the [launch script](../../../benchmark/launch.sh), set the `SIMI_HELM_CHART` environment variable to the absolute path for your local Helm chart directory.

### Simi images
In order to deploy Simi, you must publish the Simi images. They can be made with the following commands.
```bash
make simi
make simi-image
```
Once the images have been created, they can be pushed to a repository. The Helm chart can be configured to pull from the repository via the [Parameters](#parameters).

## Parameters

| Name | Description | Value |
|---|---|---|
|   `shouldFailOnError` | If true, a Simi worker pod failure will cause the job to fail.  |   `false` |
|   `isBenchmark` | If true, the Simi job will collect and return metrics on the duration each chaincode operation.  |   `true` |
|   `enableTracing` | If enabled, Jaegar traces will be enabled for the Simi job.  |   `true` |
|   `uniqueId` | The name of the Simi job.  |   `default` |
|   `targetNamespace` | If namespace of the DLN Simi is running chaincode operations on.  |   `simi` |
|   `image.repository` | The repository for the Simi image.  |   `` |
|   `image.tag` | The tag for the Simi image.  |   `` |
|   `image.consumer.repository` | The repository for the Simi consumer pod. This pod synchronizes the start of the Simi workers. This image is expected to use the same tag as the Simi image.  |   `` |
|   `image.waiter.repository` | The repository for the Simi registration waiter image.  |   `` |
|   `image.waiter.tag` | The repository for the Simi registration waiter image.  |   `` |
|   `image.publisher.repository` | The repository for the Simi metrics publisher image.  |   `` |
|   `image.publisher.tag` | The repository for the Simi metrics publisher image.  |   `` |

### Simi Resource parameters
| Name | Description | Value |
|---|---|---|
| `resources.consumer.requests.cpu` | Request CPU limit for the consumer container | 100m
| `resources.consumer.requests.memory` | Request memory limit for the consumer container | 64Mi
| `resources.simi.requests.cpu` | Request CPU limit for the Simi container | 100m
| `resources.simi.requests.memory` | Request memory limit for the Simi container | 64Mi
| `resources.simi.limits.cpu` | Resource CPU limit for the Simi container | 100m
| `resources.simi.limits.memory` | Resource memory limit for the Simi container | 64Mi

### RabbitMQ
| Name | Description | Value |
|---|---|---|
| `rabbitmq.persistence.enabled` | If enabled, the RabbitMQ container persists messages. | `false`
| `rabbitmq.fullNameOverride` | String to override name of RabbitMQ container. | `rabbitmq`
| `rabbitmq.requests.cpu` | Request CPU limit for the RabbitMQ container | 200m
| `rabbitmq.requests.memory` | Request memory limit for the RabbitMQ container | 256Mi
| `rabbitmq.service.ports.manager` | The port for the manager service of the RabbitMQ container | 15672
| `rabbitmq.auth.username` | The username for the RabbitMQ login | `rabbit`
| `rabbitmq.auth.password` | The password for the RabbitMQ login | `devRabbit2`

### InfluxDB
| Name | Description | Value |
|---|---|---|
| `influxdb.fullNameOverride` | String to override name of InfluxDB container. | `influx-simi`
| `influxdb.image.registry` | The registry for the InfluxDB image. | ``
| `influxdb.image.repository` | The repository for the InfluxDB image. | ``
| `influxdb.image.tag` | The tag for the InfluxDB image. | ``
| `influxdb.persistance.enabled` | If enabled, the InfluxDB container will persist memory. | `false`
| `influxdb.ingress.enabled` | If enabled, an ingress will be created for the InfluxDB container. | `true`
| `influxdb.auth.admin.bucket` | The bucket for Simi metrics collection. | `stats`
| `influxdb.auth.admin.org` | The organization for Simi metrics collection. | `simi`
| `influxdb.auth.admin.password` | The password for the InfluxDB admin login. | `mysecretpassword`
| `influxdb.auth.admin.token` | The password for the InfluxDB admin login. | `mysecrettoken`
| `influxdb.auth.admin.org` | The organization for Simi metrics collection. | `simi`
| `influxdb.influxdb.service.ports.http` | The port for the InfluxDB HTTP service. | `8086`
| `influxdb.influxdb.resources.limits.cpu` | Resource CPU limit for the InfluxDB container. | `1`
| `influxdb.influxdb.resources.limits.memory` | Resource memory limit for the InfluxDB container. | `2Gi`
| `influxdb.influxdb.requests.limits.cpu` | Request CPU limit for the InfluxDB container. | `1`
| `influxdb.influxdb.requests.limits.memory` | Request memory limit for the InfluxDB container. | `2Gi`

### Benchmarking Parameters
| Name | Description | Value |
|---|---|---|
|   `operationType` | The name of the chaincode operation Simi will be performing. |   `ping` |
|   `numWorkers` | The number of Simi worker pods the job will deploy. |   `1` |
|   `operationPeriod` | The time interval Simi waits between each chaincode operation. |   `5s` |

### Custom Parameters
| Name | Description | Value |
|---|---|---|
|   `operationConfig` | This object contains any of the custom configuration needed for your chaincode operation. | pingChaincode configuration |