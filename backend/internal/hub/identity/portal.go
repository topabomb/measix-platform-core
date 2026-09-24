package identity

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/portalsession"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/wire/clientapi"
)

// CanonicalPublicOrigin validates an operator-supplied public address and
// returns the one value persisted and projected by Core.
func CanonicalPublicOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || strings.HasSuffix(u.Host, ":") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", ErrInvalidInput
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", ErrInvalidInput
	}
	port := u.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", ErrInvalidInput
		}
		if (scheme == "http" && number == 80) || (scheme == "https" && number == 443) {
			port = ""
		} else {
			port = strconv.Itoa(number)
		}
	}
	hostname := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(hostname); ip != nil {
		hostname = ip.String()
	} else if strings.Contains(hostname, ":") || !validPublicHostname(hostname) {
		return "", ErrInvalidInput
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + host, nil
}

func validPublicHostname(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

// ValidatePublicOrigin accepts only the canonical value used by runtime state.
func ValidatePublicOrigin(raw string) error {
	canonical, err := CanonicalPublicOrigin(raw)
	if err != nil || canonical != raw {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) CreatePortalGrant(ctx context.Context, token string) (clientapi.PortalGrant, error) {
	p, err := s.AuthenticateAccess(ctx, token)
	if err != nil {
		return clientapi.PortalGrant{}, err
	}
	publicOrigin := s.PublicOrigin()
	if !s.PortalStaticAvailable || publicOrigin == "" || ValidatePublicOrigin(publicOrigin) != nil {
		return clientapi.PortalGrant{}, ErrPortalUnavailable
	}
	now := s.Now().UTC()
	parent, err := s.Client.Session.Get(ctx, p.SessionID)
	if err != nil {
		return clientapi.PortalGrant{}, err
	}
	ticket, err := s.Random(32)
	if err != nil {
		return clientapi.PortalGrant{}, err
	}
	id, err := s.Random(24)
	if err != nil {
		return clientapi.PortalGrant{}, err
	}
	expiry := minTime(now.Add(time.Minute), parent.ExpiresAt)
	// Expired credentials have no recovery purpose. Bound retained ephemeral rows
	// without any timer owner or plaintext token persistence.
	if _, err = s.Client.PortalSession.Delete().Where(portalsession.ExpiresAtLTE(now)).Exec(ctx); err != nil {
		return clientapi.PortalGrant{}, err
	}
	_, err = s.Client.PortalSession.Create().SetID(id).SetSessionID(p.SessionID).SetOrigin(publicOrigin).
		SetTicketDigest(security.DigestToken(ticket)).SetGrantExpiresAt(expiry).SetExpiresAt(expiry).Save(ctx)
	return clientapi.PortalGrant{ExchangeUrl: publicOrigin + "/portal/session/exchange", Ticket: ticket, ExpiresAt: expiry}, err
}

// portalParent checks live parent state, independent of an expired/rotated
// access credential. It never refreshes or updates the parent idle deadline.
func (s *Service) portalParent(ctx context.Context, sessionID string) (AccessPrincipal, time.Time, error) {
	parent, err := s.Client.Session.Get(ctx, sessionID)
	if err != nil {
		return AccessPrincipal{}, time.Time{}, ErrCredential
	}
	if parent.Channel != "ANDROID" || parent.DeviceID == nil {
		return AccessPrincipal{}, time.Time{}, ErrCredential
	}
	p := AccessPrincipal{DeploymentID: s.Signer.DeploymentID, UserID: parent.UserID, DeviceID: *parent.DeviceID, SessionID: parent.ID}
	if err := s.validateAccessPrincipal(ctx, p); err != nil {
		return AccessPrincipal{}, time.Time{}, err
	}
	return p, parent.ExpiresAt, nil
}

func (s *Service) ExchangePortalGrant(ctx context.Context, ticket string) (string, time.Time, error) {
	publicOrigin := s.PublicOrigin()
	if publicOrigin == "" || len(ticket) < 32 || len(ticket) > 128 {
		return "", time.Time{}, ErrCredential
	}
	now := s.Now().UTC()
	row, err := s.Client.PortalSession.Query().Where(portalsession.TicketDigestEQ(security.DigestToken(ticket)), portalsession.OriginEQ(publicOrigin), portalsession.ConsumedEQ(false), portalsession.RevokedEQ(false), portalsession.GrantExpiresAtGT(now)).Only(ctx)
	if ent.IsNotFound(err) {
		return "", time.Time{}, ErrCredential
	}
	if err != nil {
		return "", time.Time{}, err
	}
	_, parentExpiry, err := s.portalParent(ctx, row.SessionID)
	if err != nil {
		return "", time.Time{}, err
	}
	cookie, err := s.Random(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiry := minTime(now.Add(10*time.Minute), parentExpiry)
	n, err := s.Client.PortalSession.Update().Where(portalsession.IDEQ(row.ID), portalsession.ConsumedEQ(false), portalsession.RevokedEQ(false), portalsession.GrantExpiresAtGT(s.Now().UTC())).SetConsumed(true).SetCookieDigest(security.DigestToken(cookie)).SetExpiresAt(expiry).Save(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if n != 1 {
		return "", time.Time{}, ErrCredential
	}
	return cookie, expiry, nil
}

func (s *Service) AuthenticatePortal(ctx context.Context, cookie string) (clientapi.PortalSession, error) {
	publicOrigin := s.PublicOrigin()
	if cookie == "" || publicOrigin == "" {
		return clientapi.PortalSession{}, ErrCredential
	}
	row, err := s.Client.PortalSession.Query().Where(portalsession.CookieDigestEQ(security.DigestToken(cookie)), portalsession.ConsumedEQ(true), portalsession.RevokedEQ(false), portalsession.OriginEQ(publicOrigin), portalsession.ExpiresAtGT(s.Now().UTC())).Only(ctx)
	if ent.IsNotFound(err) {
		return clientapi.PortalSession{}, ErrCredential
	}
	if err != nil {
		return clientapi.PortalSession{}, err
	}
	p, idle, err := s.portalParent(ctx, row.SessionID)
	if err != nil {
		return clientapi.PortalSession{}, err
	}
	deployment, err := s.Client.Deployment.Get(ctx, p.DeploymentID)
	if err != nil {
		return clientapi.PortalSession{}, err
	}
	u, err := s.Client.User.Get(ctx, p.UserID)
	if err != nil {
		return clientapi.PortalSession{}, err
	}
	return clientapi.PortalSession{DeploymentId: p.DeploymentID, UserId: p.UserID, DeviceId: p.DeviceID, SessionId: p.SessionID, EnterpriseName: deployment.Name, UserDisplayName: u.DisplayName, ExpiresAt: minTime(row.ExpiresAt, idle), SessionIdleExpiresAt: idle, CsrfToken: security.CSRFToken("portal:"+cookie, s.CSRFKey)}, nil
}

func (s *Service) ClosePortal(ctx context.Context, cookie, csrf string) error {
	if _, err := s.AuthenticatePortal(ctx, cookie); err != nil {
		return err
	}
	if !security.VerifyCSRF("portal:"+cookie, csrf, s.CSRFKey) {
		return ErrNotAuthorized
	}
	_, err := s.Client.PortalSession.Update().Where(portalsession.CookieDigestEQ(security.DigestToken(cookie))).SetRevoked(true).Save(ctx)
	return err
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
