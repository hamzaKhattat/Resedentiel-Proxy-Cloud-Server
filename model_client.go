package main

// Proxy client model related functions

import (
	"database/sql"
	"fmt"
)

// Info to save
type ProxyClientInfo struct {
	Id              string
	BytesUploaded   int64 `db:"bytesUploaded"`
	BytesDownloaded int64 `db:"bytesDownloaded"`
}

// Returns info by Id
func (m *Model) GetProxyClientInfo(id string) (*ProxyClientInfo, error) {
	info := &ProxyClientInfo{}
	err := m.DB.Get(info, "SELECT * FROM proxy_clients WHERE id=?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return &ProxyClientInfo{
				Id:              id,
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
(id, bytesDownloaded, bytesUploaded) 
VALUES (:id, :bytesDownloaded, :bytesUploaded)
ON DUPLICATE KEY 
UPDATE bytesDownloaded = :bytesDownloaded, bytesUploaded = :bytesUploaded
`, info)
	if err != nil {
		return fmt.Errorf("Upsert error: %w", err)
	}
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
