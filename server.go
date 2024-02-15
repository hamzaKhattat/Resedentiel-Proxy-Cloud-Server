package main

import (
	"encoding/binary"
	"flag"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/yamux"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Common data

const (
	// Maximal number of clients per middle server
	MAX_CLIENTS_PER_MIDDLE_SERVER = 20000

	// Handshake constants
	CLNT_HS = "PFCCLHS"
	MDDL_HS = "PFCMSHS"
)

// Port number mutex
var mutex sync.Mutex

// Current port to listen on
var currentPort = 4001

// Map of active clients: port -> client
var clients sync.Map

// Map of client's host -> port
var clientsMem sync.Map

// Map of last assigned clients ports: id -> last port
var lastPorts sync.Map

// Address of the front server
var frontServerAddress string

// ID of the middle server
var middleId int

// Returns count of the active clients
func getActiveClientsCount() int {
	count := 0
	clients.Range(func(k, v interface{}) bool {
		count += 1
		return true
	})
	return count
}

type IRWBytesCounter interface {
	AddRead(n int)
	AddWritten(n int)
}

// Proxy client object
type ProxyClient struct {
	Id         string
	Port       string
	ExternalIP string
	Stream     net.Conn
	Session    *yamux.Session
	OrigConn   net.Conn
	Listener   net.Listener
	StartTime  time.Time

	HTTPServer *http.Server

	BytesRead    int64
	BytesWritten int64
}

func (pc *ProxyClient) Init() error {
	info, err := g_Model.GetProxyClientInfo(pc.Id)
	if err != nil {
		return err
	}
	pc.BytesRead = info.BytesDownloaded
	pc.BytesWritten = info.BytesUploaded
	return nil
}

func (pc *ProxyClient) saveInfo() error {
	return g_Model.SetProxyClientInfo(&ProxyClientInfo{
		Id:              pc.Id,
		BytesDownloaded: atomic.LoadInt64(&pc.BytesRead),
		BytesUploaded:   atomic.LoadInt64(&pc.BytesWritten),
	})
}

func (pc *ProxyClient) AddRead(n int) {
	atomic.AddInt64(&pc.BytesRead, int64(n))
}

func (pc *ProxyClient) AddWritten(n int) {
	atomic.AddInt64(&pc.BytesWritten, int64(n))
}

func (pc *ProxyClient) StartStatDumper() {
	go func() {
		for {
			log.Printf("")
			time.Sleep(5 * time.Second)
		}
	}()
}

var g_Model *Model

// Entry point
func main() {
	gin.SetMode(gin.ReleaseMode)
	log.Println("pfconnect-middle-server started")

	middleServerId := flag.Int("id", 1, "middle server Id, e.g. 1,2,3...")
	listenAddr := flag.String("listenAddr", "0.0.0.0:443", "listening address of the proxy server. e.g. <host>:<port>")
	externalIP := flag.String("externalIP", "108.181.201.189", "external IP of the middle server to conenct to")
	adminAddr := flag.String("adminAddr", "108.181.201.189:80", "listening address for administration, e.g. <host>:<port>")
	auth := flag.String("auth", "admin:J23490bSDJfkFH81u029d", "http auth, eg: david:hello-kitty")
	dbConnectionString := flag.String("db", "pfcserver:hADHJf10inr10f1@tcp(127.0.0.1:3306)/pfconnect?parseTime=true", "MySQL database connection string")
	flag.Parse()

	middleId = *middleServerId

	g_Model = &Model{
		DBConnectionString: *dbConnectionString,
	}
	g_Model.Init()

	adminServer := AdminServer{
		ListenAddress: *adminAddr,
		Credentials:   *auth,
		ExternalIP:    *externalIP,
	}
	adminServer.Start()

	tcpAddr, err := net.ResolveTCPAddr("tcp4", *listenAddr)
	if err != nil {
		log.Printf("Fatal error: %v", err)
		os.Exit(1)
	}
	netListen, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		log.Printf("Unable to listen: %v", err)
		os.Exit(1)
	}

	defer netListen.Close()

	log.Printf("Listening on " + *listenAddr + " for proxy clients")

	for {
		conn, err := netListen.AcceptTCP()
		if err != nil {
			log.Printf("Listening error: %v", err)
			continue
		}

		session, _ := yamux.Server(conn, nil)
		go func() {
			done := false
			for !done {
				done = handleSession(session, conn)
			}
		}()
	}
}

// Handles the client session
func handleSession(session *yamux.Session, conn net.Conn) bool {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic occured: %v", r)
		}
	}()

	stream, err := session.Accept()
	if err == nil {
		go func() {
			handleClientConnection(stream, session, conn)
			stream.Close()
		}()
		return false
	}

	if session.IsClosed() {
		clientHost := conn.RemoteAddr().String()
		clientPort, _ := clientsMem.Load(clientHost)
		if clientPort != nil {
			clientPortV := clientPort.(string)
			client, _ := clients.Load(clientPort)
			clientV := client.(*ProxyClient)
			client = client.(*ProxyClient)
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("panic occured: %v", r)
					}
				}()
				clientV.OrigConn.Close()
				listener := clientV.Listener
				listener.Close()
			}()
			clients.Delete(clientPort)
			clientsMem.Delete(clientHost)
			log.Printf("Client " + clientHost + " disconnected. Remove the port " + clientPortV)
		}
		return true
	}

	log.Printf("Got session error: %v", err)
	return false
}

var newPortLoop = 0

// Allocates new TCP port
func getNewPort() string {
	mutex.Lock()
	for {
		if newPortLoop > 1 {
			return "-1"
		}
		if _, ok := clients.Load(strconv.Itoa(currentPort)); ok {
			currentPort++
		} else {
			newPortLoop = 0
			break
		}
		if currentPort > 60000 {
			currentPort = 4001
			newPortLoop++
		}
	}
	port := currentPort
	currentPort++
	mutex.Unlock()
	return strconv.Itoa(port)
}

// Handles client connection
func handleClientConnection(conn net.Conn, session *yamux.Session, origConn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic occured: %v", r)
		}
	}()

	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Printf("%s: connection error: %v", conn.RemoteAddr().String(), err)
		conn.Close()
		return
	}

	buffer = buffer[:n]

	first := len(CLNT_HS)
	if len(buffer) < first {
		log.Printf("connection error: wrong handshake: %s", string(buffer))
		return
	}
	handshake := string(buffer[:first])
	if handshake != CLNT_HS {
		log.Printf("connection error: wrong handshake: %s", handshake)
		return
	}

	sz := buffer[first : first+4]
	clientIdSize := int(binary.LittleEndian.Uint32(sz))
	clientId := string(buffer[first+4 : first+4+clientIdSize])

	log.Printf("[INCOMNING]: New connection: Client ID: %s", clientId)

	count := getActiveClientsCount()
	if count >= MAX_CLIENTS_PER_MIDDLE_SERVER {
		log.Printf("[INCOMNING]: blocked: max clients number reached: %d", MAX_CLIENTS_PER_MIDDLE_SERVER)
		return
	}

	var port string
	v, ok := lastPorts.Load(clientId)
	if !ok {
		port = getNewPort()
		lastPorts.Store(clientId, port)
	} else {
		port = v.(string)
	}

	tcpAddr, _ := net.ResolveTCPAddr("tcp4", "0.0.0.0:"+port)
	netListen, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		log.Printf("Unable to listen on port: %v", err)
		return
	}

	defer func() {
		log.Printf("Stop listening on port (%s) for client: %s", port, conn.RemoteAddr().String())
		netListen.Close()
	}()

	log.Printf("Start listening on port (%s) for client: %s", port, conn.RemoteAddr().String())

	addr := conn.RemoteAddr().String()
	clientExternalIP, _, err := net.SplitHostPort(addr)
	if err != nil {
		clientExternalIP = addr
	}

	client := &ProxyClient{
		Id:           clientId,
		Port:         port,
		ExternalIP:   clientExternalIP,
		Stream:       conn,
		Session:      session,
		OrigConn:     origConn,
		Listener:     netListen,
		StartTime:    time.Now(),
		BytesRead:    0,
		BytesWritten: 0,
	}
	err = client.Init()
	if err != nil {
		log.Printf("Unable to initialzie client: %v", err)
		return
	}

	// Establish tunnel
	conn.Write([]byte(MDDL_HS))

	clients.Store(port, client)
	clientsMem.Store(conn.RemoteAddr().String(), port)

	log.Printf("[INCOMNING]: Connection count: %d", getActiveClientsCount())

	client.HTTPServer = &http.Server{
		Handler: &HTTPProxyHandler{
			Client: client,
		},
	}
	client.HTTPServer.Serve(netListen)

	if client, ok := clients.Load(port); ok {
		stream := client.(*ProxyClient).Stream
		if stream != nil {
			stream.Close()
			clientsMem.Delete(stream.RemoteAddr().String())
		}
	}
	clients.Delete(port)
}

