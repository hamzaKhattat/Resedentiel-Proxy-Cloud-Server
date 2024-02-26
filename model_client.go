
package main

// Proxy client model related functions

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/go-sql-driver/mysql"

)

// Info to save
type ProxyClientInfo struct {
	Id              string
	Pcip            string	`db:"pcip"`
	Port		string	`db:"port"`
	Serverip	string	`db:"serverip"`
	Username	string	`db:"username"`
	Password	string	`db:"password"`
	BytesUploaded   int64 `db:"bytesUploaded"`
	BytesDownloaded int64 `db:"bytesDownloaded"`
}
type usersbyip struct{
	Username	string	
	Password	string	
}

// Returns info by Id
func (m *Model) GetProxyClientInfo(id string) (*ProxyClientInfo, error) {
	info := &ProxyClientInfo{}
	err := m.DB.Get(info, "SELECT * FROM proxy_clients WHERE id=?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return &ProxyClientInfo{
				Id:              id,
				Pcip:		"",
				Port:		"",
				Serverip:	"",
				Username: 	"",
				Password:	"",
				BytesUploaded:   0,
				BytesDownloaded: 0,
			}, nil
		}
		return nil, fmt.Errorf("Select error: %w", err)
	}
	return info, nil
}

// Uperts the client info
func (m *Model) SetProxyClientInfo(info *ProxyClientInfo) error {
	
	_, err := m.DB.NamedExec(`
INSERT INTO proxy_clients 
(id,pcip, port, serverip, username, password, bytesDownloaded, bytesUploaded) 
VALUES (:id, :pcip, :port, :serverip, :username, :password,:bytesDownloaded, :bytesUploaded)
ON DUPLICATE KEY 
UPDATE bytesDownloaded = :bytesDownloaded, bytesUploaded = :bytesUploaded , pcip = :pcip
`, info)
	if err != nil {
		return fmt.Errorf("Upsert error: %w", err)
	}  
	fmt.Println("Saving info client in DB ...............")
	us,errr:=m.GetUsersByip(info.Pcip)
	if(errr!=nil){fmt.Println("Fuck off...")}
	fmt.Println(us)
	fmt.Println("User:",us.Username,"Pass:",us.Password)
	return nil
}

// Returns all info items
func (m *Model) GetProxyClientsInfo() ([]*ProxyClientInfo, error) {
	info := []*ProxyClientInfo{}
	err := m.DB.Select(&info, "SELECT * FROM proxy_clients")
	if err != nil {
		return nil, err
	}
	return info, nil
}
//Return info from ip
func (m *Model) GetUsersByip(ip string) (usersbyip, error) {
    var user usersbyip
    err := m.DB.QueryRow(fmt.Sprintf("SELECT username, password FROM proxy_clients WHERE pcip='%s'", ip)).Scan(&user.Username, &user.Password)
    if err != nil {
        log.Printf("Error while getting info from IP %s: %s", ip, err)
        return usersbyip{}, err
    }

    fmt.Printf("Getting info from IP: %s\nUsername: %s\nPassword: %s\n", ip, user.Username, user.Password)

    return user, nil
}

