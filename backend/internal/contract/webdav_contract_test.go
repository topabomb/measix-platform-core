package contract_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestDAVDefinitionAndUserPermissionContracts(t *testing.T) {
	for _, file := range []string{"api/admin/admin.openapi.yaml", "api/client/client-control.openapi.yaml"} {
		doc := loadContractDoc(t, file)
		def := doc.Components.Schemas["WebDAVDefinition"]
		if def == nil {
			t.Fatalf("%s lacks DAV resource definition", file)
		}
		data, err := os.ReadFile("../../../api/fixtures/workspace/webdav-definition.json")
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err = json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		if err = def.Value.VisitJSON(value); err != nil {
			t.Fatal(err)
		}
		value["clientProtocol"] = "MCP_STREAMABLE_HTTP"
		if def.Value.VisitJSON(value) == nil {
			t.Fatal("DAV accepted MCP protocol")
		}
		permission := doc.Components.Schemas["DAVAccessMode"]
		if permission == nil {
			t.Fatal("missing DAV permission")
		}
		for _, valid := range []string{"READ_ONLY", "FULL"} {
			if err := permission.Value.VisitJSON(valid); err != nil {
				t.Fatal(err)
			}
		}
		for _, invalid := range []any{"ADMIN", "", nil, true} {
			if permission.Value.VisitJSON(invalid) == nil {
				t.Fatalf("accepted permission %#v", invalid)
			}
		}
	}
}

func TestDAVControlBindingRequiresPermissionAndIndependentIdentity(t *testing.T) {
	doc := loadContractDoc(t, "api/internal/relay-control.openapi.yaml")
	schema := doc.Components.Schemas["UserDAVBinding"]
	if schema == nil {
		t.Fatal("missing explicit DAV binding")
	}
	data, err := os.ReadFile("../../../api/fixtures/workspace/webdav-binding.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if err = schema.Value.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	delete(value, "accessMode")
	if schema.Value.VisitJSON(value) == nil {
		t.Fatal("missing permission accepted")
	}
	value["accessMode"] = "FULL"
	value["resourceId"] = "mcp_550e8400-e29b-41d4-a716-446655440000"
	if schema.Value.VisitJSON(value) == nil {
		t.Fatal("MCP identity accepted as DAV")
	}
}
