package models

import (
	"time"
)

// MachineAccess represents access permissions for a user on a machine
type MachineAccess struct {
	MachineID  string    `json:"machineId"`
	UserID     string    `json:"userId"`
	Permission string    `json:"permission"`
	GrantedAt  time.Time `json:"grantedAt"`
}
