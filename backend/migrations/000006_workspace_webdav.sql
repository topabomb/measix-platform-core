-- Additive DAV resource/control state. No remote writes or Release rewrites.
ALTER TABLE workspace_services ADD COLUMN dav_resource_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_services ADD COLUMN dav_runtime_route_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_services ADD COLUMN dav_staged BOOLEAN NOT NULL DEFAULT 0;
UPDATE workspace_services SET
 dav_resource_id = 'dav_' || lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()%4)+1,1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
 dav_runtime_route_id = 'rte_' || lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()%4)+1,1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)));
CREATE UNIQUE INDEX workspace_services_dav_resource_id ON workspace_services(dav_resource_id) WHERE dav_resource_id <> '';
CREATE UNIQUE INDEX workspace_services_dav_runtime_route_id ON workspace_services(dav_runtime_route_id) WHERE dav_runtime_route_id <> '';

ALTER TABLE agent_spaces ADD COLUMN file_access_intent TEXT NOT NULL DEFAULT 'UNCONFIRMED';
ALTER TABLE agent_spaces ADD COLUMN dav_access_mode TEXT NOT NULL DEFAULT 'FULL';
ALTER TABLE agent_spaces ADD COLUMN dav_binding_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_spaces ADD COLUMN dav_applied_access_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_spaces ADD COLUMN dav_applied_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workspace_operations ADD COLUMN dav_binding_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE request_usages ADD COLUMN http_method TEXT NOT NULL DEFAULT '';
ALTER TABLE request_usages ADD COLUMN statistics_only BOOL NOT NULL DEFAULT false;

-- A pending explicit revoke is already a deny intent. Preserve it even while
-- the previous secret reference still exists; recovery must not reauthorize it.
UPDATE agent_spaces SET file_access_intent = CASE
 WHEN (SELECT action FROM workspace_operations
       WHERE user_id=agent_spaces.id AND action IN ('SET_DAV','REVOKE_DAV')
       ORDER BY created_at DESC,rowid DESC LIMIT 1) = 'REVOKE_DAV' THEN 'REVOKED'
 WHEN (SELECT action FROM workspace_operations
       WHERE user_id=agent_spaces.id AND action IN ('SET_DAV','REVOKE_DAV')
       ORDER BY created_at DESC,rowid DESC LIMIT 1) = 'SET_DAV' THEN 'AUTHORIZED'
 WHEN dav_confirmed=1 OR (dav_secret_id IS NOT NULL AND dav_secret_id<>'' AND dav_secret_version>0) THEN 'AUTHORIZED'
 ELSE 'UNCONFIRMED' END;
