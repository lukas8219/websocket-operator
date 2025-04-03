package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"lukas8219/websocket-operator/internal/route"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var (
	router route.RouterImpl
)

func InitializeLoadBalancer(mode string) {
	router = route.NewRouter(route.RouterConfig{Mode: route.RouterConfigMode(mode)})
	router.InitializeHosts()
}

type ConnectionTracker struct {
	upstreamHost           string
	upstreamConn           net.Conn
	downstreamConn         net.Conn
	cancelUpstream         context.CancelFunc
	upstreamContext        context.Context
	upstreamCancellingChan chan int
}

func main() {
	port := flag.String("port", "3000", "Port to listen on")
	mode := flag.String("mode", "kubernetes", "Mode to use")
	flag.Parse()
	InitializeLoadBalancer(*mode)
	log.Printf("Starting load balancer server on port %s", *port)
	http.ListenAndServe("0.0.0.0:"+*port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("ws-user-id")
		if user == "" {
			log.Println("No user id provided")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Println("New connection from", user)

		host := router.Route(user)
		if host == "" {
			log.Println("No host found for user", user)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		clientConn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		connectionTracker := &ConnectionTracker{
			upstreamHost:   host,
			downstreamConn: clientConn,
		}

		connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
		proxiedCon, err := connectToUpstreamAndProxyMessages(connectionTracker, user)
		connectionTracker.upstreamConn = proxiedCon
		connectionTracker.upstreamCancellingChan = make(chan int, 1)

		router.OnHostRebalance(func(hosts [][2]string) error {
			log.Println("OnHostRebalance", hosts)
			for _, affectedHost := range hosts {
				oldHost := affectedHost[0]
				newHost := affectedHost[1]
				log.Println("Checking host", oldHost, "versus", connectionTracker.upstreamHost)
				if connectionTracker.upstreamHost == oldHost {
					log.Println("Rebalancing connection from", oldHost, "to", newHost)
					connectionTracker.cancelUpstream()
					connectionTracker.upstreamHost = newHost

					log.Println("Waiting for upstream to cancel", oldHost)
					select {
					case <-connectionTracker.upstreamCancellingChan:
						log.Println("Successfully received cancellation signal")
					case <-time.After(5 * time.Second):
						log.Println("Timeout waiting for upstream cancellation, proceeding anyway")
					}

					log.Println("Rebalancing host", host, "to", newHost)
					connectionTracker.upstreamContext, connectionTracker.cancelUpstream = context.WithCancel(context.Background())
					connectionTracker.upstreamCancellingChan = make(chan int, 1)
					_, err = connectToUpstreamAndProxyMessages(connectionTracker, user)
					if err != nil {
						log.Println(errors.Join(err, errors.New("failed to reconnect to upstream")))
						connectionTracker.Close()
						return err
					}
					break
				}
			}
			return nil
		})
		go handleIncomingMessagesToProxy(connectionTracker)
	}))
}

func (c *ConnectionTracker) Close() {
	if c.upstreamConn != nil {
		c.upstreamConn.Close()
	}
	if c.downstreamConn != nil {
		c.downstreamConn.Close()
	}
}

func handleIncomingMessagesToProxy(connectionTracker *ConnectionTracker) {
	for {
		clientConnection := connectionTracker.downstreamConn
		upstreamConnection := connectionTracker.upstreamConn
		msg, op, err := wsutil.ReadClientData(clientConnection)
		if err != nil {
			log.Println(errors.Join(err, errors.New("failed to read from downstream")))
			return
		}
		select {
		case <-connectionTracker.upstreamContext.Done():
			log.Println("Upstream context done")
			return
		default:
			//TODO: SEGFAULT here in case upstreamConnection is nil - due to failure or whatever. network delays could cause this
			err = wsutil.WriteClientMessage(upstreamConnection, op, msg)
			if err != nil {
				log.Println(errors.Join(err, errors.New("failed to write to upstream")))
				return
			}
			if op == ws.OpClose {
				log.Println("downstream server closed connection")
				return
			}
		}
	}
}

// TODO
func connectToUpstreamAndProxyMessages(connectionTracker *ConnectionTracker, user string) (net.Conn, error) {
	host := connectionTracker.upstreamHost
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"ws-user-id": []string{user},
		},
	}
	log.Println("Dialing upstream", host)
	proxiedConn, _, _, err := dialer.Dial(connectionTracker.upstreamContext, "ws://"+host)
	if err != nil {
		log.Println(errors.Join(errors.New("failed to dial upstream"), err))
		return nil, err
	}
	log.Println("Connected to upstream", host)
	connectionTracker.upstreamConn = proxiedConn

	waitSignal := func() {
		log.Println("Closing upstream connection", host)
		select {
		case connectionTracker.upstreamCancellingChan <- 1:
			log.Println("Successfully signaled cancellation")
		default:
			log.Println("No one waiting for cancellation signal, skipping")
		}
	}

	//Missing DEFER
	proxySidecarServerToClient := func() {
		defer waitSignal()
		for {
			select {
			case <-connectionTracker.upstreamContext.Done():
				log.Println("Upstream context done", host)
				proxiedConn.Close()
				return
			default:
				//Read as client - from the server.
				msg, op, err := wsutil.ReadServerData(proxiedConn)
				if err != nil {
					log.Println(errors.Join(err, errors.New("failed to read from upstream"+host)))
					return
				}
				//Write as client - to the proxied connection
				err = wsutil.WriteServerMessage(connectionTracker.downstreamConn, op, msg)
				if err != nil {
					log.Println(errors.Join(err, errors.New("failed to write to downstream")))
					return
				}
				if op == ws.OpClose {
					log.Println("upstream server closed connection", host)
					return
				}
			}
		}
	}
	go proxySidecarServerToClient()
	return proxiedConn, nil
}
