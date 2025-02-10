# benchmark

The benchmark package benchmarks operations and logs them upon completion of the Simi job.

## Build prerequisites
* [protoc](http://google.github.io/proto-lens/installing-protoc.html)
* `protoc-gen-go`: `go get -u github.com/golang/protobuf/protoc-gen-go`

## Updating message
After editing the .proto file, run `go generate`
