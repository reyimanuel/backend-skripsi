package constants

import "time"

//
// Timeout constants for external services
var (
	// ExternalServiceTimeout is the default timeout for external service calls
	ExternalServiceTimeout = 5 * time.Second
	
	// FCMTimeout is the timeout for Firebase Cloud Messaging calls
	FCMTimeout = 5 * time.Second
	
	// EmailTimeout is the timeout for email service calls
	EmailTimeout = 8 * time.Second
	
	// DatabaseQueryTimeout is the timeout for database queries
	DatabaseQueryTimeout = 10 * time.Second
)
