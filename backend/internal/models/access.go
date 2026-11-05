package models

import (
	"time"
)

// HostAccess represents access permissions for a user on a host
type HostAccess struct {
	HostID     string    `json:"hostId"`
	UserID     string    `json:"userId"`
	Permission string    `json:"permission"`
	GrantedAt  time.Time `json:"grantedAt"`
}
