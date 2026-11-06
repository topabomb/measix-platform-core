package migrations

import (
	"context"
	"measix/platform/internal/common/sqliteutil"
	"path/filepath"
	"testing"
)

func TestReleaseHistoryUpgradeCapturesNamesAndGenerationBeforePurge(t *testing.T) {
	ctx := context.Background()
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = applySet(ctx, db, List()[:3]); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO deployments(id,name,status,created_at,updated_at) VALUES('dep_old','old','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO users(id,username,display_name,role,status,created_at,updated_at) VALUES('usr_old','old','Old','ADMIN','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO managed_states(id,active_managed_generation,managed_state_revision,desired_control_revision,runtime_status,updated_at) VALUES('current',28,109,54,'READY',CURRENT_TIMESTAMP)`,
		`INSERT INTO managed_releases(id,managed_generation,status,release_content_json,snapshot_json,snapshot_hash,source_draft_revision,created_by_user_id,created_at) VALUES('rel_old',31,'ACTIVATION_FAILED','{"bindings":[]}','{"models":[{"modelId":"mdl_old","displayName":"Original name"}]}','original-hash',58,'usr_old',CURRENT_TIMESTAMP)`,
		`INSERT INTO activations(id,kind,state,idempotency_key,request_hash,control_revision,bundle_hash,target_generation,target_descriptor_json,created_by_user_id,created_at) VALUES('act_old','PUBLISH','FAILED','idem_old','original-request',55,'original-bundle',32,'{}','usr_old',CURRENT_TIMESTAMP)`,
		`INSERT INTO upstreams(id,name,config_revision,status,created_at,updated_at) VALUES('ups_old','old',1,'ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_requests(id,request_hash,deployment_id,user_id,capability,resource_id,client_protocol,upstream_id,managed_generation,control_revision,mode,source,decision_json,state,admitted_at,updated_at) VALUES('req_old','original-request','dep_old','usr_old','MODEL','mdl_old','OPENAI_CHAT_COMPLETIONS','ups_old',31,55,'UNLIMITED','DEFAULT','{}','RECONCILIATION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	}
	for _, s := range statements {
		if _, err = db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var high int
	var name, hash string
	if err = db.QueryRow(`SELECT last_assigned_generation FROM managed_states`).Scan(&high); err != nil || high != 32 {
		t.Fatalf("high water=%d %v", high, err)
	}
	if err = db.QueryRow(`SELECT resource_display_name,request_hash FROM budget_requests`).Scan(&name, &hash); err != nil || name != "Original name" || hash != "original-request" {
		t.Fatalf("admission=%s %s %v", name, hash, err)
	}
	if _, err = db.Exec(`DELETE FROM managed_releases`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT resource_display_name FROM budget_requests`).Scan(&name); err != nil || name != "Original name" {
		t.Fatal("history cleanup erased late settlement context")
	}
	if _, err = Verify(ctx, db); err != nil {
		t.Fatal(err)
	}
}
