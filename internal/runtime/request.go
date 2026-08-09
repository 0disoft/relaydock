package runtime

import "time"

type RequestRecord struct {
	ID              string
	TenantID        string
	ProjectID       string
	VirtualKeyID    string
	IngressProtocol string
	VirtualModel    string
	Status          string
	CreatedAt       time.Time
}
