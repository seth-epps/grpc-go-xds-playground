# test statshandler with xds
## Run grpc-server
start the grpc server at https://github.com/seth-epps/hello-go 

## Start the xds management server
`go run xds/main.go` will start an xds management server at `localhost:18000`

## Run the test
`go run .` will run a request against the grpc-server directly and through the configured xds server

export `GRPC_GO_LOG_VERBOSITY_LEVEL=99` and `GRPC_GO_LOG_SEVERITY_LEVEL=info` to see the xds logging