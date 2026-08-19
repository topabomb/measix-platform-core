package identity

import "time"

type BootstrapResult struct {
	DeploymentID string
	AdminUserID  string
	DraftID      string
	PolicyID     string
}

type EnrollmentGrant struct {
	EnrollmentID string
	Code         string
	ExpiresAt    time.Time
}

type ExchangeResult struct {
	DeploymentID         string
	UserID               string
	DeviceID             string
	SessionID            string
	AccessToken          string
	AccessTokenExpiresAt time.Time
	RefreshToken         string
	RefreshExpiresAt     time.Time
}

type AccessPrincipal struct {
	DeploymentID string
	UserID       string
	DeviceID     string
	SessionID    string
}

type AdminSessionResult struct {
	UserID       string
	DisplayName  string
	Role         string
	CookieSecret string
	CSRFToken    string
	ExpiresAt    time.Time
}
