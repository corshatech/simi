# pingChaincode Fabric Chaincode Operation Example

Simi is by default configured with the `pingChaincode` fabric operation as an example of creating a `WorkerConfig` instance. 

The `PingWorker` defined in [./ping.go](./ping.go), performs a chaincode operation like this:
```go
func pingChaincode(_ shim.ChaincodeStubInterface, _ []string) peer.Response {
	return shim.Success(nil)
}
```

For more information on how to write chaincode for a Hyperledger Fabric DLN see the [docs](https://hyperledger-fabric.readthedocs.io/en/release-2.5/chaincode4ade.html).