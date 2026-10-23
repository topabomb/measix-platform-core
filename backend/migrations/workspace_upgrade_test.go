package migrations

import (
	"context"
	"measix/platform/internal/common/sqliteutil"
	"path/filepath"
	"testing"
)

func TestWorkspaceUpgradePreservesPopulatedLegacyAttribution(t *testing.T) {
	ctx := context.Background()
	db, e := sqliteutil.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = applySet(ctx, db, List()[:1]); e != nil {
		t.Fatal(e)
	}
	statements := []string{
		`INSERT INTO deployments(id,name,status,created_at,updated_at) VALUES('dep_old','legacy','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO users(id,username,display_name,role,status,created_at,updated_at) VALUES('usr_old','legacy','Legacy','USER','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO upstreams(id,name,config_revision,status,created_at,updated_at) VALUES('ups_old','legacy',1,'ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_requests(id,request_hash,deployment_id,user_id,capability,resource_id,client_protocol,upstream_id,managed_generation,control_revision,mode,source,decision_json,state,admitted_at,updated_at) VALUES('req_old','original-admission-hash','dep_old','usr_old','MCP','mcp_old','MCP_STREAMABLE_HTTP','ups_old',7,11,'UNLIMITED','DEFAULT',x'7b226f6c64223a747275657d','SETTLED',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_settlements(request_id,revision,payload_hash,meters_json,complete,source,outcome,reported_by,created_at) VALUES('req_old',1,'original-settlement-hash',x'5b5d',1,'RELAY','SETTLED','relay',CURRENT_TIMESTAMP)`,
		`INSERT INTO request_usages(request_id,deployment_id,user_id,resource_id,resource_kind,client_protocol,runtime_route_id,upstream_id,managed_generation,control_revision,started_at,completed_at,forwarded,http_status,request_bytes,response_bytes,duration_ms,request_completeness,settlement_state,settlement_revision,budget_revision,ingested_at) VALUES('req_old','dep_old','usr_old','mcp_old','MCP','MCP_STREAMABLE_HTTP','rte_old','ups_old',7,11,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,1,200,123,456,89,'COMPLETE','SETTLED',1,0,CURRENT_TIMESTAMP)`,
	}
	for _, q := range statements {
		if _, e = db.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = Apply(ctx, db); e != nil {
		t.Fatalf("populated upgrade: %v", e)
	}
	var hash, upstream, raw string
	if e = db.QueryRow(`SELECT request_hash,upstream_id,hex(decision_json) FROM budget_requests WHERE id='req_old' AND workspace_target_json IS NULL`).Scan(&hash, &upstream, &raw); e != nil {
		t.Fatal(e)
	}
	if hash != "original-admission-hash" || upstream != "ups_old" || raw != "7B226F6C64223A747275657D" {
		t.Fatalf("legacy admission changed %s %s %s", hash, upstream, raw)
	}
	var quantity int
	if e = db.QueryRow(`SELECT response_bytes FROM request_usages WHERE request_id='req_old' AND upstream_id='ups_old' AND workspace_target_json IS NULL`).Scan(&quantity); e != nil || quantity != 456 {
		t.Fatalf("usage changed: %d %v", quantity, e)
	}
	if e = db.QueryRow(`SELECT payload_hash FROM budget_settlements WHERE request_id='req_old'`).Scan(&hash); e != nil || hash != "original-settlement-hash" {
		t.Fatalf("settlement changed: %s %v", hash, e)
	}
	rows, e := db.Query(`PRAGMA foreign_key_check`)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("upgrade broke a foreign key")
	}
	if _, e = Verify(ctx, db); e != nil {
		t.Fatal(e)
	}
}
