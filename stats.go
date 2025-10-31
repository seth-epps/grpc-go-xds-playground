// stolen from 
// https://github.com/grpc/grpc-go/blob/363018c3d687d40153225f283301d3e491e7c5a4/examples/features/stats_monitoring/statshandler/handler.go
package main

import (
	"context"
	"log"
	"net"
	"path/filepath"

	"google.golang.org/grpc/stats"
)

type Handler struct{}

type connStatCtxKey struct{}

func (st *Handler) TagConn(ctx context.Context, stat *stats.ConnTagInfo) context.Context {
	log.Printf("[TagConn] [%T]: %+[1]v", stat)
	return context.WithValue(ctx, connStatCtxKey{}, stat)
}

func (st *Handler) HandleConn(ctx context.Context, stat stats.ConnStats) {
	var rAddr net.Addr
	if s, ok := ctx.Value(connStatCtxKey{}).(*stats.ConnTagInfo); ok {
		rAddr = s.RemoteAddr
	}

	if stat.IsClient() {
		log.Printf("[server addr: %s] [HandleConn] [%T]: %+[2]v", rAddr, stat)
	} else {
		log.Printf("[client addr: %s] [HandleConn] [%T]: %+[2]v", rAddr, stat)
	}
}

type rpcStatCtxKey struct{}

func (st *Handler) TagRPC(ctx context.Context, stat *stats.RPCTagInfo) context.Context {
	log.Printf("[TagRPC] [%T]: %+[1]v", stat)
	return context.WithValue(ctx, rpcStatCtxKey{}, stat)
}

func (st *Handler) HandleRPC(ctx context.Context, stat stats.RPCStats) {
	var sMethod string
	if s, ok := ctx.Value(rpcStatCtxKey{}).(*stats.RPCTagInfo); ok {
		sMethod = filepath.Base(s.FullMethodName)
	}

	var cAddr net.Addr
	if s, ok := ctx.Value(connStatCtxKey{}).(*stats.ConnTagInfo); ok {
		cAddr = s.RemoteAddr
	}

	if stat.IsClient() {
		log.Printf("[server method: %s] [HandleRPC] [%T]: %+[2]v", sMethod, stat)
	} else {
		log.Printf("[client addr: %s] [HandleRPC] [%T]: %+[2]v", cAddr, stat)
	}
}
