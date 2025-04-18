package main

import (
	"flag"
	"lukas8219/websocket-operator/cmd/loadbalancer/server"
	"lukas8219/websocket-operator/internal/logger"
	"lukas8219/websocket-operator/internal/route"
)

var (
	router route.RouterImpl
)

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	debug := flag.Bool("debug", false, "Debug mode")
	flag.Parse()
	logger.SetupLogger(*debug)
	router = route.NewRouter(route.RouterConfig{Mode: route.RouterConfigMode(*mode)})
	router.InitializeHosts()
	server.StartServer(server.ServerConfig{
		Router: router,
		Port:   *port,
	})
}
