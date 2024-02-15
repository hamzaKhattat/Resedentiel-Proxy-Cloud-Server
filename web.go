package main

// Middle server admin page related functions

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"encoding/json"
	"os"
	"sort"
	"strings"
)

type Location struct {
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
	Region      string `json:"region"`
}

func locat_Country(ipAddress string) string {
	// Specify the IP address for which you want to get the location information

	// Make a GET request to the GeoIP API with the specified IP address
	resp, err := http.Get("http://ip-api.com/json/" + ipAddress)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Decode the JSON response into a Location struct
	var location Location
	if err := json.NewDecoder(resp.Body).Decode(&location); err != nil {
		log.Fatal(err)
	}

	// Print the location information
	return location.Country

}

func locat_City(ipAddress string) string {
	// Specify the IP address for which you want to get the location information

	// Make a GET request to the GeoIP API with the specified IP address
	resp, err := http.Get("http://ip-api.com/json/" + ipAddress)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Decode the JSON response into a Location struct
	var location Location
	if err := json.NewDecoder(resp.Body).Decode(&location); err != nil {
		log.Fatal(err)
	}

	// Print the location information
	return location.City

}
// Admin web server object
type AdminServer struct {
	ListenAddress string
	Credentials   string
	ExternalIP    string
}

// Starts the admin server
func (s *AdminServer) Start() {
	go func() {
		http.HandleFunc("/proxies", s.getProxyServers)
		http.HandleFunc("/users", s.getProxyUsers)
		log.Printf("Listening on " + s.ListenAddress + " for HTTP administration")
		err := http.ListenAndServe(s.ListenAddress, nil)
		if err != nil {
			log.Printf("Fatal error: %v", err.Error())
			os.Exit(1)
		}
	}()
}

// Authenticates admin user before accessing the panels
func (s *AdminServer) authProxyPanel(w http.ResponseWriter, r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if !ok || s.Credentials != u+":"+p {
		w.Header().Add("WWW-Authenticate", "Basic realm=\"*\"")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(nil)
		return true
	}
	return false
}

// Admin pages common HTML header
var header = `
<html>
<head>
	<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@4.0.0/dist/css/bootstrap.min.css" crossorigin="anonymous">
	<style>
		.online {
			color: #299b09;
		}
		.offline {
			color: #e14a3b;
		}
		.users .table {
			max-width: 400px;
		}
		.hdr {
			max-width: 960px;
			margin: auto;
		}
		.container {
			max-width: 950px;
			padding-top: 14px;
		}
		.btn {
			margin-top: -2px;
		}
	</style>
</head>
<body>

<script src="https://code.jquery.com/jquery-3.2.1.slim.min.js" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/popper.js@1.12.9/dist/umd/popper.min.js" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/bootstrap@4.0.0/dist/js/bootstrap.min.js" crossorigin="anonymous"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/clipboard.js/1.4.0/clipboard.min.js" crossorigin="anonymous"></script>
<script>
window.addEventListener('load', function () {
	let clipboard = new Clipboard('.btn')
	clipboard.on('success', function (e) {
  	  e.clearSelection()
	})
})
</script>
<div class="hdr d-flex flex-column flex-md-row align-items-center p-3 px-md-4 mb-3 bg-white border-bottom box-shadow">
      <h5 class="my-0 mr-md-auto font-weight-normal">Admin</h5>
      <nav class="my-2 my-md-0 mr-md-3">
        <a class="p-2 text-dark" href="/proxies">Proxies</a>
        <a class="p-2 text-dark" href="/users">Users</a>
      </nav>
</div>
<div class="container">
`

// Client info descriptor
type ClientDescriptor struct {
	Info   *ProxyClientInfo  // DB record
	Client *ProxyClient 		 // Active client
}

// Renders proxies page
func (s *AdminServer) getProxyServers(w http.ResponseWriter, r *http.Request) {

	if s.authProxyPanel(w, r) {
		return
	}

	infoItems, err := g_Model.GetProxyClientsInfo()
	if err != nil {
		io.WriteString(w, fmt.Sprintf("error: %v", err))
		return
	}

	result := header + `
<h3>Proxies:</h3>
<table class="table">
<tbody>
<tr>
	<th>Id</th>
	<th>IP</th>
	<th>Proxy IP</th>
	<th>Transfer, Kb</th>
	<th>Country</th>
	<th>City</th>
</tr>
`
	clientsById := make(map[string]*ProxyClient)
	clients.Range(func(port, v interface{}) bool {
		client := v.(*ProxyClient)
		clientsById[client.Id] = client
		return true
	})

	var rows []*ClientDescriptor
	for _, info := range infoItems {
		rows = append(rows, &ClientDescriptor{
			Info:   info,
			Client: clientsById[info.Id],
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Info.Id < rows[j].Info.Id
	})

	for _, row := range rows {
		class := "offline"
		externalIP := "offline"
		proxyAddress := "offline"
		button := ""
		elementId := "addr-" + row.Info.Id
		country:="offline"
		city:="offline"
		if row.Client != nil {
			class = "online"
			externalIP = row.Client.ExternalIP
			proxyAddress = s.ExternalIP + ":" + row.Client.Port
			button = fmt.Sprintf(`<button class="btn btn-sm" data-clipboard-target="#%s">Copy</button>`, elementId)
			country=locat_Country(externalIP)
			city=locat_City(externalIP)
		}

		result += fmt.Sprintf(`
<tr class="%s">
	<td>%s</td>
	<td>%s</td>
	<td>
		<span id="%s">%s</span>
		`+button+`
	</td>

	<td>%d / %d</td>
	<td>%s</td>
	<td>%s</td>
</tr>`, class,
			row.Info.Id,
			externalIP,
			elementId, proxyAddress,
			int64(row.Info.BytesUploaded/1024), int64(row.Info.BytesDownloaded/1024),
			country,
			city)

	}

	result += "</table></div></body></html>"

	io.WriteString(w, result)
}

// Renders users page
func (s *AdminServer) getProxyUsers(w http.ResponseWriter, r *http.Request) {

	if s.authProxyPanel(w, r) {
		return
	}

	users := g_Model.GetUsers()

	result := header + `
<div class="users">
<h3>Proxy Users:</h3>
<table class="table">
<tbody>
<tr>
	<th width="100">Id</th>
	<th>Login</th>
	<th>Password</th>
</tr>
`
	for _, user := range users {
		result += fmt.Sprintf(`
<tr>
	<td>%d</td>
	<td>%s</td>
	<td>%s</td>
</tr>`, user.Id, user.Login, strings.TrimPrefix(user.Password, "!"))
	}

	result += "</table></div></div></body></html>"

	io.WriteString(w, result)
}

