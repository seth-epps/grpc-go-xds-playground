package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	manager "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
)

const (
	xdsPort = 18000
	nodeID  = "test-node"
)

type loggingAdapter struct {
	*slog.Logger
}

func (la *loggingAdapter) Debugf(format string, args ...any) {
	la.Debug(fmt.Sprintf(format, args...))
}

func (la *loggingAdapter) Errorf(format string, args ...any) {
	la.Error(fmt.Sprintf(format, args...))
}

func (la *loggingAdapter) Warnf(format string, args ...any) {
	la.Warn(fmt.Sprintf(format, args...))
}

func (la *loggingAdapter) Infof(format string, args ...any) {
	la.Info(fmt.Sprintf(format, args...))
}

func main() {
	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)
	slog.SetDefault(logger)
	configCache := cache.NewSnapshotCache(false, cache.IDHash{}, &loggingAdapter{logger})

	clusterName := "service_cluster"
	endpointAddr := "0.0.0.0"
	var endpointPort uint32 = 9090
	xdsAddr := "listener"


	edsEndpoint := &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpoint.LocalityLbEndpoints{
			{
				Locality: &core.Locality{Zone: "local"},
				LoadBalancingWeight: wrapperspb.UInt32(1),
				LbEndpoints: []*endpoint.LbEndpoint{
					{
						HostIdentifier: &endpoint.LbEndpoint_Endpoint{
							Endpoint: &endpoint.Endpoint{
								Address: &core.Address{
									Address: &core.Address_SocketAddress{
										SocketAddress: &core.SocketAddress{
											Protocol: core.SocketAddress_TCP,
											Address: endpointAddr ,
											PortSpecifier: &core.SocketAddress_PortValue{
												PortValue: endpointPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cdsCluster := &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       &durationpb.Duration{Seconds: 5},
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_EDS},
		EdsClusterConfig: &cluster.Cluster_EdsClusterConfig{
			EdsConfig: &core.ConfigSource{
				ConfigSourceSpecifier: &core.ConfigSource_Ads{
					Ads: &core.AggregatedConfigSource{},
				},
			},
			ServiceName: clusterName,
		},
	}

	routerpb, _ := anypb.New(&routerv3.Router{})
	httpConnectionManager := &manager.HttpConnectionManager{
		HttpFilters: []*manager.HttpFilter{
			{
				Name: wellknown.Router,
				ConfigType: &manager.HttpFilter_TypedConfig{
					TypedConfig: routerpb,
				},
			},
		},
		RouteSpecifier: &manager.HttpConnectionManager_Rds{
			Rds: &manager.Rds{
				ConfigSource: &core.ConfigSource{
					ConfigSourceSpecifier: &core.ConfigSource_Ads{
						Ads: &core.AggregatedConfigSource{},
					},
				},
				RouteConfigName: "local_route",
			},
		},
	}

	httpConnectionManagerpb, _ := anypb.New(httpConnectionManager)

	ldsListener := &listener.Listener{
		Name: xdsAddr,
		FilterChains: []*listener.FilterChain{
			{
				Filters: []*listener.Filter{
					{
						Name: xdsAddr,
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: httpConnectionManagerpb,
						},
					},
				},
			},
		},
		ApiListener: &listener.ApiListener{
			ApiListener: httpConnectionManagerpb,
		},
	}

	rdsRoute := &route.RouteConfiguration{
		Name: "local_route",
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "local",
				Domains: []string{"*"},
				Routes: []*route.Route{
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_WeightedClusters{
									WeightedClusters: &route.WeightedCluster{
										Clusters: []*route.WeightedCluster_ClusterWeight{
											{
												Name:   clusterName,
												Weight: wrapperspb.UInt32(100),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	snapshot, err := cache.NewSnapshot("1",
		map[resource.Type][]types.Resource{
			resource.ClusterType:  {cdsCluster},
			resource.ListenerType: {ldsListener},
			resource.RouteType:    {rdsRoute},
			resource.EndpointType: {edsEndpoint},
		},
	)
	if err != nil {
		log.Fatalf("failed to create snapshot: %v", err)
	}

	// This doesn't use the default node id / constant hash so you actually need to specify 
	// the node in the xds bootstrap
	if err := configCache.SetSnapshot(context.Background(), nodeID, snapshot); err != nil {
		log.Fatalf("failed to set snapshot: %v", err)
	}

	xdsServer := server.NewServer(context.Background(), configCache, &callbacks{})
	grpcServer := grpc.NewServer()
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsServer)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(xdsPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("xDS server listening on %v", lis.Addr())
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down xDS server...")
	grpcServer.GracefulStop()
}

// callbacks implements the go-control-plane server.Callbacks interface (optional for basic testing)
type callbacks struct{}

func (c *callbacks) OnStreamOpen(context.Context, int64, string) error        { return nil }
func (c *callbacks) OnStreamClosed(int64, *core.Node)                         {}
func (c *callbacks) OnStreamRequest(int64, *discovery.DiscoveryRequest) error { return nil }
func (c *callbacks) OnStreamResponse(context.Context, int64, *discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {
}
func (c *callbacks) OnFetchRequest(context.Context, *discovery.DiscoveryRequest) error         { return nil }
func (c *callbacks) OnFetchResponse(*discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {}
func (c *callbacks) OnDeltaStreamOpen(context.Context, int64, string) error                    { return nil }
func (c *callbacks) OnDeltaStreamClosed(int64, *core.Node)                                     {}
func (c *callbacks) OnStreamDeltaRequest(int64, *discovery.DeltaDiscoveryRequest) error        { return nil }
func (c *callbacks) OnStreamDeltaResponse(int64, *discovery.DeltaDiscoveryRequest, *discovery.DeltaDiscoveryResponse) {
}
