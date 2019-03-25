package server

import (
	"crypto/rand"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Hucaru/Valhalla/game"
	"github.com/Hucaru/Valhalla/mpacket"

	"github.com/Hucaru/Valhalla/constant"
	"github.com/Hucaru/Valhalla/database"
	"github.com/Hucaru/Valhalla/mnet"
)

type loginServer struct {
	config   loginConfig
	dbConfig dbConfig
	eRecv    chan *mnet.Event
	wg       *sync.WaitGroup
	state    game.Login
}

func NewLoginServer(configFile string) *loginServer {
	config, dbConfig := loginConfigFromFile("config.toml")

	ls := &loginServer{
		eRecv:    make(chan *mnet.Event),
		config:   config,
		dbConfig: dbConfig,
		wg:       &sync.WaitGroup{},
	}

	return ls
}

func (ls *loginServer) Run() {
	log.Println("Login Server")

	ls.establishDatabaseConnection()

	ls.wg.Add(1)
	go ls.acceptNewClientConnections()

	ls.wg.Add(1)
	go ls.acceptNewServerConnections()

	ls.wg.Add(1)
	go ls.processEvent()

	ls.wg.Wait()
}

func (ls *loginServer) establishDatabaseConnection() {
	database.Connect(ls.dbConfig.User, ls.dbConfig.Password, ls.dbConfig.Address, ls.dbConfig.Port, ls.dbConfig.Database)
	go database.Monitor()
}

func (ls *loginServer) acceptNewServerConnections() {
	defer ls.wg.Done()

	listener, err := net.Listen("tcp", ls.config.ServerListenAddress+":"+ls.config.ServerListenPort)

	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	log.Println("Server listener ready:", ls.config.ServerListenAddress+":"+ls.config.ServerListenPort)

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Println("Error in accepting client", err)
			close(ls.eRecv)
			return
		}

		serverConn := mnet.NewServer(conn, ls.eRecv, ls.config.PacketQueueSize)

		go serverConn.Reader()
		go serverConn.Writer()
	}
}

func (ls *loginServer) acceptNewClientConnections() {
	defer ls.wg.Done()

	records, err := database.Handle.Query("UPDATE accounts SET isLogedIn=?", 0)

	defer records.Close()

	if err != nil {
		panic(err)
	}

	log.Println("Reset all accounts login server status")

	listener, err := net.Listen("tcp", ls.config.ClientListenAddress+":"+ls.config.ClientListenPort)

	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	log.Println("Client listener ready:", ls.config.ClientListenAddress+":"+ls.config.ClientListenPort)

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Println("Error in accepting client", err)
			close(ls.eRecv)
			return
		}

		keySend := [4]byte{}
		rand.Read(keySend[:])
		keyRecv := [4]byte{}
		rand.Read(keyRecv[:])

		loginConn := mnet.NewClient(conn, ls.eRecv, ls.config.PacketQueueSize, keySend, keyRecv)

		go loginConn.Reader()
		go loginConn.Writer()

		conn.Write(game.PacketClientHandshake(constant.MapleVersion, keyRecv[:], keySend[:]))
	}
}

func (ls *loginServer) processEvent() {
	defer ls.wg.Done()

	for {
		select {
		case e, ok := <-ls.eRecv:

			if !ok {
				log.Println("Stopping event handling due to channel read error")
				return
			}

			loginConn, ok := e.Conn.(mnet.Client)

			if ok {
				switch e.Type {
				case mnet.MEClientConnected:
					log.Println("New client from", loginConn)
				case mnet.MEClientDisconnect:
					log.Println("Client at", loginConn, "disconnected")
					loginConn.Cleanup()
				case mnet.MEClientPacket:
					ls.state.HandleClientPacket(loginConn, mpacket.NewReader(&e.Packet, time.Now().Unix()))
				}
			} else {
				serverConn, ok := e.Conn.(mnet.Server)

				if ok {
					switch e.Type {
					case mnet.MEServerConnected:
						log.Println("New server from", serverConn)
					case mnet.MEServerDisconnect:
						log.Println("Server at", serverConn, "disconnected")
					case mnet.MEServerPacket:
						ls.state.HandleServerPacket(serverConn, mpacket.NewReader(&e.Packet, time.Now().Unix()))
					}
				}
			}

		}
	}
}
