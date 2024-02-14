package main

// Data model related functions

import (
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"log"
	"sync"
	"time"
)

// Main model object
type Model struct {
	DBConnectionString string

	DB           *sqlx.DB
	usersByLogin sync.Map
}

// Initializes model
func (m *Model) Init() {
	// Connect to MySQL DB
	var err error
	m.DB, err = sqlx.Connect("mysql", m.DBConnectionString)
	if err != nil {
		log.Fatal(err)
	}
	m.DB.SetConnMaxLifetime(time.Minute * 5)
	m.DB.SetMaxIdleConns(5)
	m.DB.SetMaxOpenConns(5)

	// Load proxy users
	m.LoadUsers()
 
    // Start proxy clients info dumper
	m.startInfoDumper()
}

// Loads proxy users from DB
func (m *Model) LoadUsers() error {
	var users []*User
	err := m.DB.Select(&users, "SELECT * FROM proxy_users")
	if err != nil {
		return err
	}
	for _, user := range users {
		m.usersByLogin.Store(user.Login, user)
	}
	return nil
}

var EAUTH_WRONG_USER = errors.New("Wrong user")
var EAUTH_WRONG_CREDENTIALS = errors.New("Wrong credentials")

// Authenticates proxy user by login and password pair
func (m *Model) Auth(login string, password string) (*User, error) {
	user := m.GetUserByLogin(login)
	if user != nil {
		if user.Auth(password) {
			return user, nil
		}
		return nil, EAUTH_WRONG_CREDENTIALS
	}
	return nil, EAUTH_WRONG_USER
}

// Returns proxy users by login
func (m *Model) GetUserByLogin(login string) *User {
	u, ok := m.usersByLogin.Load(login)
	if ok {
		return u.(*User)
	}
	return nil
}

// Start proxy clients info dumper
func (m *Model) startInfoDumper() {
	go func() {
		for {
			// Iterate active clients
			clients.Range(func(k, v interface{}) bool {
				pc := v.(*ProxyClient)
				log.Printf("[DUMPER]: saving info of client: %s...", pc.Id)
				err := pc.saveInfo()
				if err != nil {
					log.Printf("[DUMPER]: info saving error: %v", err)
				}
				return true
			})
			// Dump every 5 seconds
			time.Sleep(5 * time.Second)
		}
	}()
}

// Returns array of all active users
func (m *Model) GetUsers() []*User {
	var users []*User
	m.usersByLogin.Range(func(k, v interface{}) bool {
		user := v.(*User)
		users = append(users, user)
		return true
	})
	return users
}
