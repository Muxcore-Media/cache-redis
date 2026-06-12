module github.com/Muxcore-Media/cache-redis

go 1.26.4

require (
	github.com/Muxcore-Media/core v0.4.0
	github.com/Muxcore-Media/core/pkg/contracts v0.0.0
	github.com/Muxcore-Media/core/sdk/go/module v0.1.0
	github.com/redis/go-redis/v9 v9.20.1
	google.golang.org/grpc v1.81.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260610212136-7ab31c22f7ad // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/Muxcore-Media/core => ../core

replace github.com/Muxcore-Media/core/sdk/go/module => ../core/sdk/go/module

replace github.com/Muxcore-Media/core/pkg/contracts => ../core/pkg/contracts
