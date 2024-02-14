package main

// Proxy user model related functional

import (
	"golang.org/x/crypto/bcrypt"
)

// Proxy user object
type User struct {
	Id       int
	Login    string
	Password string
}

// Authenticates the proxy user with the specified password
func (u *User) Auth(password string) bool {
	// Plain text password marked with "!" prefix
	if u.Password[0:1] == "!" {
		pass := u.Password[1:len(u.Password)]
		if password == pass {
			return true
		}
	} else {
		return checkPasswordHash(password, u.Password)
	}
	return false
}

// Compares hash of passowrd with the specified hash`
func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
