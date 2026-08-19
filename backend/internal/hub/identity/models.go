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

// UserView and DeviceView are application-layer read models used by HTTP/UI boundaries.
// They intentionally keep Ent entities inside the identity service.
type UserView struct {
	ID          string
	Username    string
	DisplayName string
	Role        string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type DeviceView struct {
	ID             string
	UserID         string
	InstallationID string
	AppVersion     *string
	LastSeenAt     *time.Time
	Status         string
}

type AdminPrincipalView struct {
	UserID      string
	DisplayName string
	Role        string
	SessionID   string
	ExpiresAt   time.Time
}
