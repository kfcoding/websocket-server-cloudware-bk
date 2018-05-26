package handler

import (
	"net/http"
	"log"
	"github.com/gorilla/websocket"
	"strings"
	"net/url"
	"sync"
)

var (
	tunnels    = make(map[string]*Tunnel)
	cloudwares = make(map[string]int)
	lock       sync.Mutex
	upgrader   = websocket.Upgrader{}
)

func Upgrade(w http.ResponseWriter, r *http.Request) {
	var (
		client *websocket.Conn
		tunnel *Tunnel
		ok     bool
		err    error
	)

	// get tunnel
	paths := strings.Split(r.URL.Path, "/")
	token := paths[len(paths)-1]
	if tunnel, ok = tunnels[token]; !ok {
		log.Printf("handleSession: can't find session '%s'", token)
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
		}
	}()
	tunnel.Timer <- true
	if tunnel, ok = tunnels[token]; !ok {
		log.Printf("handleSession: can't find session '%s'", token)
		return
	}

	// get client conn
	client, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	// get pulsar conn
	u := url.URL{Scheme: "ws://", Host: tunnel.PodIP + ":9800", Path: ""}
	pulsar, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		client.Close()
		log.Fatal("dial pulsar : ", err)
	}

	// bind client and pulsar conn
	tunnel.Client = client
	tunnel.Pulsar = pulsar
	go tunnel.Iocopy()
	go tunnel.Iocopy2()
}

func addToTunnels(pod, token string, tunnel *Tunnel) {
	lock.Lock()
	tunnels[token] = tunnel
	log.Print(cloudwares[pod])
	cloudwares[pod] = cloudwares[pod] + 1
	lock.Unlock()
}

func deleteFromTunnels(pod, token string) {
	lock.Lock()
	delete(tunnels, token)
	cloudwares[pod] = cloudwares[pod] - 1
	if cloudwares[pod] <= 0 {
		delete(cloudwares, pod)
	}
	lock.Unlock()
}

func Run(done chan string) {
	for {
		select {
		case token := <-done:
			if tunnel, ok := tunnels[token]; ok {
				deleteFromTunnels(token, tunnel.Pod)
				log.Print("delete tunnel succeed!")
			} else {
				log.Print("delete tunnel error!")
			}
		}
	}
}
