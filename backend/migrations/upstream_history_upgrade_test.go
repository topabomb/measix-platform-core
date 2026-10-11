package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"measix/platform/ent"
	"measix/platform/internal/common/sqliteutil"
	"measix/platform/internal/hub/upstream"
)

func TestUpstreamDeletionAfterPopulatedHistoryUpgrade(t *testing.T) {
	ctx := context.Background()
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = applySet(ctx, db, List()[:4]); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO deployments(id,name,status,created_at,updated_at) VALUES('dep_old','legacy','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO users(id,username,display_name,role,status,password_hash,created_at,updated_at) VALUES('usr_old','legacy','Legacy','ADMIN','ACTIVE','synthetic-hash',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO upstreams(id,name,config_revision,active_config_revision,status,created_at,updated_at) VALUES('ups_old','legacy',1,1,'ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO upstream_config_revisions(upstream_id,revision,config_json,created_by_user_id,created_at) VALUES('ups_old',1,x'7b7d','usr_old',CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_requests(id,request_hash,deployment_id,user_id,capability,resource_id,resource_display_name,client_protocol,upstream_id,managed_generation,control_revision,mode,source,decision_json,state,admitted_at,updated_at) VALUES('req_old','original-admission-hash','dep_old','usr_old','MCP','mcp_old','Original MCP','MCP_STREAMABLE_HTTP','ups_old',7,11,'UNLIMITED','DEFAULT',x'7b226f6c64223a747275657d','RECONCILIATION',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_requests(id,request_hash,deployment_id,user_id,capability,resource_id,resource_display_name,client_protocol,workspace_target_json,managed_generation,control_revision,mode,source,decision_json,state,admitted_at,updated_at) VALUES('req_workspace','workspace-admission','dep_old','usr_old','MCP','mcp_workspace','Original workspace','MCP_STREAMABLE_HTTP',x'7b7d',7,11,'UNLIMITED','DEFAULT',x'7b7d','SETTLED',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_settlements(request_id,revision,payload_hash,meters_json,complete,source,outcome,reported_by,created_at) VALUES('req_old',1,'original-settlement-hash',x'5b5d',1,'RELAY','RECONCILIATION','relay',CURRENT_TIMESTAMP)`,
		`INSERT INTO budget_reconciliations(request_id,state,reason,opened_at) VALUES('req_old','OPEN','Original reason',CURRENT_TIMESTAMP)`,
		`INSERT INTO request_usages(id,request_id,deployment_id,user_id,resource_id,resource_kind,client_protocol,runtime_route_id,upstream_id,managed_generation,control_revision,started_at,completed_at,forwarded,http_status,request_bytes,response_bytes,duration_ms,request_completeness,settlement_state,settlement_revision,budget_revision,ingested_at) VALUES(90,'req_old','dep_old','usr_old','mcp_old','MCP','MCP_STREAMABLE_HTTP','rte_old','ups_old',7,11,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,1,200,123,456,89,'COMPLETE','RECONCILIATION',1,0,CURRENT_TIMESTAMP)`,
		`INSERT INTO pricing_rules(id,resource_id,upstream_id,meter,unit_size,unit_price_decimal,currency,effective_from) VALUES('pr_old','mcp_old','ups_old','REQUESTS','1','0.125','CNY',CURRENT_TIMESTAMP)`,
		`UPDATE sqlite_sequence SET seq=150 WHERE name='request_usages'`,
	}
	for _, q := range statements {
		if _, err = db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	tables := []string{"budget_requests", "request_usages", "pricing_rules", "budget_settlements", "budget_reconciliations"}
	before := map[string]string{}
	for _, table := range tables {
		before[table] = historyTableBytes(t, db, table)
	}
	if _, err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err = upstream.NewService(client, nil).DeleteUpstream(ctx, "usr_old", "ups_old", 1); err != nil {
		t.Fatalf("historical attribution must not prevent deleting an unused connection: %v", err)
	}
	for _, table := range tables {
		if got := historyTableBytes(t, db, table); got != before[table] {
			t.Fatalf("%s historical bytes changed", table)
		}
	}
	var count, seq int
	if err = db.QueryRow(`SELECT COUNT(*) FROM upstream_config_revisions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("operational revisions: %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name='request_usages'`).Scan(&seq); err != nil || seq != 150 {
		t.Fatalf("usage high water: %d %v", seq, err)
	}
	// Late settlement still references its original admission after connection deletion.
	if _, err = db.Exec(`INSERT INTO budget_settlements(request_id,revision,payload_hash,meters_json,complete,source,outcome,reported_by,created_at) VALUES('req_old',2,'late-settlement',x'5b5d',1,'RELAY','SETTLED','relay',CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE budget_requests SET state='SETTLED',last_settlement_revision=2 WHERE id='req_old'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE budget_requests SET user_id='missing' WHERE id='req_old'`); err == nil {
		t.Fatal("user foreign key was removed")
	}
	if _, err = db.Exec(`UPDATE budget_requests SET upstream_id=NULL WHERE id='req_old'`); err == nil {
		t.Fatal("exclusive target constraint was removed")
	}
	if _, err = db.Exec(`INSERT INTO budget_settlements(request_id,revision,payload_hash,meters_json,complete,source,outcome,reported_by,created_at) VALUES('missing',1,'bad',x'5b5d',1,'RELAY','SETTLED','relay',CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("admission foreign key was removed")
	}
	if _, err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("migration left broken foreign keys")
	}
}

func historyTableBytes(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	// Column ordering is canonical because rebuilding a table can change physical order.
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY name`, table)
	if err != nil {
		t.Fatal(err)
	}
	columns := ""
	for rows.Next() {
		var col string
		if err = rows.Scan(&col); err != nil {
			t.Fatal(err)
		}
		if columns != "" {
			columns += ","
		}
		columns += `"` + col + `"`
	}
	rows.Close()
	rows, err = db.Query("SELECT " + columns + " FROM " + table + " ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names, _ := rows.Columns()
	var records [][]any
	for rows.Next() {
		values := make([]any, len(names))
		args := make([]any, len(names))
		for i := range args {
			args[i] = &values[i]
		}
		if err = rows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		records = append(records, values)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
