package identity_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/identity"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func newService(t *testing.T)(*identity.Service,string){t.Helper();st:=testutil.OpenStore(t);_,privateKey,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)};deploymentID:=platformid.New(platformid.Deployment);signer,err:=security.NewAccessSigner(privateKey,deploymentID,"test-key",10*time.Minute);if err!=nil{t.Fatal(err)};s:=identity.New(st.Client,signer,[]byte("01234567890123456789012345678901"));now:=time.Date(2026,8,19,0,0,0,0,time.UTC);s.Now=func()time.Time{return now};signer.Now=s.Now;return s,deploymentID}

func TestI1IdentityEnrollmentRefreshAndRevoke(t *testing.T){ctx:=context.Background();s,deploymentID:=newService(t);boot,err:=s.Bootstrap(ctx,"Example Corp","Root.Admin","Root Admin","correct horse battery staple");if err!=nil{t.Fatal(err)};if boot.DeploymentID!=deploymentID||boot.AdminUserID==""||boot.DraftID==""{t.Fatalf("invalid bootstrap result: %+v",boot)};member,err:=s.CreateUser(ctx," Alice ","Alice","MEMBER");if err!=nil{t.Fatal(err)};if member.Username!="alice"{t.Fatalf("normalized username=%q",member.Username)};if _,err:=s.CreateUser(ctx,"ALICE","Other","MEMBER");!errors.Is(err,identity.ErrConflict){t.Fatalf("duplicate username err=%v",err)};grant,err:=s.CreateEnrollment(ctx,member.ID,boot.AdminUserID,10*time.Minute);if err!=nil{t.Fatal(err)};exchange,err:=s.ExchangeEnrollment(ctx,grant.Code,platformid.New(platformid.Installation),"1.0.0");if err!=nil{t.Fatal(err)};if exchange.UserID!=member.ID||exchange.DeploymentID!=deploymentID||exchange.RefreshToken==""||exchange.AccessToken==""{t.Fatalf("invalid exchange: %+v",exchange)};if _,err:=s.ExchangeEnrollment(ctx,grant.Code,platformid.New(platformid.Installation),"1.0.0");!errors.Is(err,identity.ErrAlreadyUsed){t.Fatalf("second exchange err=%v",err)};if _,_,err:=s.Refresh(ctx,exchange.RefreshToken);err!=nil{t.Fatalf("refresh: %v",err)};if err:=s.RevokeDevice(ctx,exchange.DeviceID);err!=nil{t.Fatal(err)};if _,_,err:=s.Refresh(ctx,exchange.RefreshToken);!errors.Is(err,identity.ErrRevoked){t.Fatalf("refresh after revoke err=%v",err)}}
func TestAdminSessionCookieAndCSRF(t *testing.T){ctx:=context.Background();s,_:=newService(t);if _,err:=s.Bootstrap(ctx,"Example Corp","admin","Admin","correct horse battery staple");err!=nil{t.Fatal(err)};login,err:=s.LoginAdmin(ctx," ADMIN ","correct horse battery staple");if err!=nil{t.Fatal(err)};if _,_,err:=s.AuthenticateAdmin(ctx,login.CookieSecret,"wrong",true);!errors.Is(err,identity.ErrNotAuthorized){t.Fatalf("wrong csrf err=%v",err)};u,_,err:=s.AuthenticateAdmin(ctx,login.CookieSecret,login.CSRFToken,true);if err!=nil||u.Role!="ADMIN"{t.Fatalf("authenticate user=%v err=%v",u,err)};if err:=s.LogoutAdmin(ctx,login.CookieSecret);err!=nil{t.Fatal(err)};if _,_,err:=s.AuthenticateAdmin(ctx,login.CookieSecret,login.CSRFToken,false);!errors.Is(err,identity.ErrNotAuthorized){t.Fatalf("session remained active: %v",err)}}
