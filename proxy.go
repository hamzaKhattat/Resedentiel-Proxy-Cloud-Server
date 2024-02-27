package main

// Middle server proxy related functions

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// Send HTTP 407 "Proxy Authentication Required" error 
func (c *RequestContext) send407() {
	c.W.Header().Add("Proxy-Authenticate", "Basic realm=\"PF Connect\"")
	c.W.Header().Set("Connection", "Close")
	c.W.Header().Set("Proxy-Connection", "Close")
	http.Error(c.W, "Proxy authentication required", http.StatusProxyAuthRequired)
}

// Decodes base64-encoded username:password pair
func decodeCreds(auth string) (user, pass string, ok bool) {
	auth = strings.TrimSpace(auth)
	enc := base64.StdEncoding
	buf := make([]byte, enc.DecodedLen(len(auth)))
	n, err := enc.Decode(buf, []byte(auth))
	if err != nil {
		return "", "", false
	}
	auth = string(buf[:n])

	colon := strings.Index(auth, ":")
	if colon == -1 {
		return "", "", false
	}

	return auth[:colon], auth[colon+1:], true
}





// Authenticate proxy user
func (c *RequestContext) Authenticate(ip string) bool {
        authHeaders := c.R.Header["Proxy-Authorization"]
        for _, auth := range authHeaders {

                login, pass, ok := decodeCreds(strings.TrimPrefix(auth, "Basic "))	
		//fmt.Printf("Before okk .. testing %s:%s for %s...\n",login,pass,ip)
                if ok {
                        user,err:= g_Model.GetUsersByip(ip)
                        if err!=nil{
                     fmt.Println("Error Getting infos from ip in proxy.go .",err)					
				c.send407()
				return true
                        }
                       // fmt.Println("Trying Authenticate with ",login,pass)
                        if login==user.Username{
                             //   fmt.Println("Testing Authenticate with ",login)
                                if pass==user.Password{
                                        fmt.Println("Correct!!!!!!!!!!!!")
					c.User=user
                                        return false
                                }else{
									fmt.Println("Invalid Password !!!!!!!!!!!")
									c.send407()
									return true
                                }
                        }else{
							log.Printf("Bad auth: (%s/%s): %v", login, pass, err)
							c.send407()
							return true
                }
//                      user, err := g_Model.Auth(ip,login, pass)
//                      if err == nil {
//                              c.User = user
//                              return false
//                      }
                        
        }}
        log.Printf("No auth provided")
        c.send407()
        return true

	}
// HTTP proxy handler object
type HTTPProxyHandler struct {
	Client *ProxyClient  // keeps related active client in context
}

var hopByHop = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// removeHopByHopHeaders removes header fields listed in
// http://tools.ietf.org/html/draft-ietf-httpbis-p1-messaging-14#section-7.1.3.1
func removeHopByHopHeaders(h http.Header) {
	toRemove := hopByHop
	if c := h.Get("Connection"); c != "" {
		for _, key := range strings.Split(c, ",") {
			toRemove = append(toRemove, strings.TrimSpace(key))
		}
	}
	for _, key := range toRemove {
		h.Del(key)
	}
}

// HTTP proxy handler
func (h *HTTPProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	defer func() {
		if r := recover(); r != nil {
			log.Printf("ServeHTTP panic occured: %v", r)
		}
	}()

	// Initialize context
	c := &RequestContext{
		H: h,
		R: req,
		W: rw,
	}

	//log.Printf("ServeHTTP: R: %+v", *req)
	fmt.Println("Trying to Authenticate for ",h.Client.ExternalIP)
	if c.Authenticate(h.Client.ExternalIP) {
		log.Printf("Authentication failed")
		return
	}

	log.Printf("Authenticated as: %s", c.User.Username)

	// Handle HTTP CONNECT method
	if c.R.Method == "CONNECT" {
		c.handleTnnl()
		return
	}

	// Handle other HTTP methods

	log.Printf("HTTP %s: to: %s ...", c.R.Method, c.R.Host)

	log.Println("Incomming header:",c.R.Header)

	dialer := ClientDialer{
		Client: h.Client,
	}
	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	outReq := new(http.Request)
	*outReq = *req

	removeHopByHopHeaders(c.R.Header)

	res, err := transport.RoundTrip(outReq)
	if err != nil {
		rw.WriteHeader(http.StatusBadGateway)
		return
	}

	for key, value := range res.Header {
		for _, v := range value {
			rw.Header().Add(key, v)
		}
	}

	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
	res.Body.Close()

}

// CustomDialer implements the net.Dialer interface to establish a connection with a proxy server
type ClientDialer struct {
	Client *ProxyClient
}

// Dial is called by the HTTP client to establish a connection with the proxy server
func (d *ClientDialer) Dial(network, addr string) (net.Conn, error) {

	clientConn := d.Client.Stream
	session := d.Client.Session

	conn, err := session.OpenStream()
	if err != nil {
		e := fmt.Errorf("Failed to create new stream to client: %s -> %w", clientConn.RemoteAddr().String(), err)
		log.Printf("OpenStream: error: %v", e)
		return nil, e
	}

	log.Printf("Dial: connecting to: %s (%s) ...", addr, network)

	// Send the network and addr prefixed by their length
	networkLength := byte(len(network))

	_, err = conn.Write([]byte{networkLength})
	if err != nil {
		conn.Close()
		return nil, err
	}
	_, err = conn.Write([]byte(network))
	if err != nil {
		conn.Close()
		return nil, err
	}
	addrLength := byte(len(addr))
	_, err = conn.Write([]byte{addrLength})
	if err != nil {
		conn.Close()
		return nil, err
	}
	_, err = conn.Write([]byte(addr))
	if err != nil {
		conn.Close()
		return nil, err
	}

	log.Printf("Reading status")

	reply := make([]byte, 2) // Assuming 'OK' is 2 bytes long
	_, err = conn.Read(reply)
	if err != nil {
		conn.Close()
		return nil, err
	}

	log.Printf("The status is: %s", string(reply))

	if string(reply) != "OK" {
		conn.Close()
		return nil, fmt.Errorf("Client is unable to establish the connection: %s", string(reply))
	}

	countingConn := &CountingConn{
		Conn:    conn,
		counter: d.Client,
	}

	return countingConn, nil
}

// Counting connection counts transferred bytes by calling
// an external counter passed by interface
type CountingConn struct {
	net.Conn
	counter IRWBytesCounter
}

func (c *CountingConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.counter.AddRead(n)
	return n, err
}

func (c *CountingConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	c.counter.AddWritten(n)
	return n, err
}

// Handles the HTTP tunnel connection
func (c *RequestContext) handleTnnl() {

	defer func() {
		if r := recover(); r != nil {
			log.Printf("HTTP CONNECT: panic occured: %v", r)
		}
	}()

	host := c.R.URL.Host

	log.Printf("HTTP CONNECT: to: %s ...", host)
	log.Println("Incomming header:",c.R.Header["Host"])

	clientConn, err := newhjkCnn(c.W)
	if err != nil {
		log.Printf("HTTP CONNECT: unable to hijack connection: %v", err)
		http.Error(c.W, "Internal server error", http.StatusInternalServerError)
		return
	}

	dialer := ClientDialer{
		Client: c.H.Client,
	}
	serverConn, err := dialer.Dial("tcp", host)
	if err != nil {
		log.Printf("HTTP CONNECT: Unable to connect to: %s -> %v", host, err.Error())
		clientConn.Write([]byte("HTTP/1.0 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer serverConn.Close()

	log.Printf("HTTP CONNECT: connected to: %s", host)

	clientConn.Write([]byte("HTTP/1.0 200 Connection Established\r\n\r\n"))

	go io.Copy(serverConn, clientConn)
	io.Copy(clientConn, serverConn)
}

type RequestContext struct {
	RequestId string
	H         *HTTPProxyHandler
	R         *http.Request
	W         http.ResponseWriter
	Resp      *http.Response
	User      usersbyip
}

// Connection that has been hijacked (to fulfill a CONNECT request)
type hjkCnn struct {
	net.Conn
	io.Reader
}

func (hc *hjkCnn) Read(b []byte) (int, error) {
	return hc.Reader.Read(b)
}

func newhjkCnn(w http.ResponseWriter) (*hjkCnn, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("connection doesn't support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	err = bufrw.Flush()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &hjkCnn{
		Conn:   conn,
		Reader: bufrw.Reader,
	}, nil
}
