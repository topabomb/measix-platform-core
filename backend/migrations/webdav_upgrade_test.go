package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"measix/platform/internal/common/sqliteutil"
	"measix/platform/pkg/platformid"
)

func TestDAVUpgradePreservesPreviousWorkspaceAuthorization(t *testing.T) {
	ctx := context.Background()
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "previous.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = applySet(ctx, db, List()[:3]); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO workspace_services(id,name,config_revision,active_config_revision,enabled,state,mcp_server_id,runtime_route_id,created_at,updated_at) VALUES('wss_old','Existing',2,2,1,'ACTIVE','mcp_old','rte_old',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO agent_spaces(id,workspace_service_id,remote_username,agent_space_id,binding_revision,intent,state,dav_secret_id,dav_secret_version,dav_confirmed,remote_active,stop_pending,created_at,updated_at) VALUES('usr_active','wss_old','existing_a','spc_a',7,'CONNECTED','CONNECTED','sec_original',3,1,1,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('usr_revoked','wss_old','existing_b','spc_b',9,'CONNECTED','CONNECTED','sec_b',2,0,1,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('usr_unknown','wss_old','existing_c','spc_c',11,'DISCONNECTED','DISCONNECTED',NULL,NULL,0,0,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO workspace_operations(id,workspace_service_id,user_id,action,idempotency_key,request_hash,config_revision,binding_revision,target_json,state,step,created_by_user_id,created_at,updated_at) VALUES('wop_revoke','wss_old','usr_revoked','REVOKE_DAV','idem_original','original_hash',2,9,x'7b7d','UNKNOWN','DAV_REVOKE_SENT','admin',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	} {
		if _, err = db.ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	for user, intent := range map[string]string{"usr_active": "AUTHORIZED", "usr_revoked": "REVOKED", "usr_unknown": "UNCONFIRMED"} {
		var actual, mode string
		var revision int64
		if err = db.QueryRow(`SELECT file_access_intent,dav_access_mode,binding_revision FROM agent_spaces WHERE id=?`, user).Scan(&actual, &mode, &revision); err != nil {
			t.Fatal(err)
		}
		if actual != intent || mode != "FULL" || revision < 7 {
			t.Fatalf("%s: %s/%s/%d", user, actual, mode, revision)
		}
	}
	var dav, route, secret, target string
	var version int
	if err = db.QueryRow(`SELECT dav_resource_id,dav_runtime_route_id FROM workspace_services WHERE id='wss_old'`).Scan(&dav, &route); err != nil {
		t.Fatal(err)
	}
	if platformid.Validate(platformid.WebDAV, dav) != nil || platformid.Validate(platformid.Route, route) != nil {
		t.Fatal("invalid new resource identities", dav, route)
	}
	if err = db.QueryRow(`SELECT dav_secret_id,dav_secret_version,agent_space_id FROM agent_spaces WHERE id='usr_active'`).Scan(&secret, &version, &target); err != nil {
		t.Fatal(err)
	}
	if secret != "sec_original" || version != 3 || target != "spc_a" {
		t.Fatal("upgrade changed original credential or space")
	}
	if result, err := Apply(ctx, db); err != nil || len(result.Applied) != 0 {
		t.Fatalf("repeat upgrade: %+v %v", result, err)
	}
	var repeated string
	if err = db.QueryRow(`SELECT dav_resource_id FROM workspace_services WHERE id='wss_old'`).Scan(&repeated); err != nil || repeated != dav {
		t.Fatal("resource identity changed on replay", err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("broken foreign key after DAV upgrade")
	}
}
