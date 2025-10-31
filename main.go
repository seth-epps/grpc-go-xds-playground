package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/seth-epps/hello-go/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/xds"
)

var xdsBootstrap string = `{
	"xds_servers": [
		{
			"server_uri": "localhost:18000",
			"channel_creds": [
				{"type": "insecure"}
			],
			"server_features": [
				"xds_v3",
				"trusted_xds_server"
			]
		}
	],
	"node": {
		"id": "test-node",
		"locality": {
			"zone": "local"
		}
	}
}`

func main() {
	call()
	log.Printf("\n************************\n------------------------\n************************\n")
	callXds()
}

func call() {
	log.Println("Running with normal grpc client")
	target := "localhost:9090"

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(&Handler{}),
	}

	if err := doCall(target, opts); err != nil {
		log.Fatal(err)
	}
}

func callXds() {
	log.Println("Running with xds client")
	target := "xds:///listener"

	resolver, err := xds.NewXDSResolverWithConfigForTesting([]byte(xdsBootstrap))
	if err != nil {
		log.Fatalf("failed to configure resolver: %v", err)
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(&Handler{}),
		grpc.WithResolvers(resolver),
	}

	if err = doCall(target, opts); err != nil {
		log.Fatal(err)
	}
}

func doCall(target string, opts []grpc.DialOption) error {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to server %q: %w", target, err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := protos.NewHelloClient(conn)

	resp, err := c.SayHello(ctx, &protos.HelloRequest{})
	if err != nil {
		return fmt.Errorf("unexpected error from SayHello: %w", err)
	}
	log.Printf("RPC response: %s", *resp.Message)
	return nil
}
