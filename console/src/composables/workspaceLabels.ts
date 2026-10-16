const labels: Record<string,string> = {
 remote_not_ready:'远程服务尚未就绪，请稍后检查',remote_route_unavailable:'管理地址未提供所需接口，请检查服务地址',invalid_remote_response:'管理服务返回的结果无法确认，请核对服务配置',remote_rejected:'管理服务拒绝请求，请检查配置',
 workspace_targets_missing:'部分原有空间不存在，请核对部署及备份；不能自动替换现有空间',workspace_target_mismatch:'远端空间身份与原绑定不一致，请核对服务地址',workspace_target_unconfirmed:'原空间身份未确认，请先处理待核实操作',workspace_recreation_needs_attention:'部分开通结果需要核实，请到对应用户工作区处理',
 dav_not_published:'WebDAV 资源尚未启用并发布',dav_activation_pending:'权限等待服务确认',
 CONNECTED:"已开通",DISCONNECTED:"已断开",CONNECTING:"开通中",RESTORING:"恢复中",DISCONNECTING:"断开中",DELETING:"删除中",UNPROVISIONED:"未开通",
 revocation_requires_remote_confirmation:'撤销待核实：原创建或启用请求可能仍在远端执行，请确认结束后继续',
 not_provisioned:'尚未开通', available:'可用', workspace_not_connected:'连接尚未就绪', workspace_service_unavailable:'远程工作区服务未启用', user_unavailable:'用户已禁用或删除', mcp_not_published:'MCP 尚未发布（可选，不影响文件管理）', dav_credential_unavailable:'DAV 凭据不可用，请核实上游配置后重新签发', dav_not_configured:'未配置文件服务',
 SAVED:'已保存，待应用', APPLYING:'应用中', ACTIVE:'已生效', DISABLING:'停用收尾中', DISABLED:'已停用', NEEDS_ATTENTION:'需要处理', UNKNOWN:'结果未知，需要核实', PENDING:'等待处理', RUNNING:'处理中', COMPLETED:'已完成',
 dav_confirmation_mismatch:'签发响应与配置不一致，请核对文件服务的对外地址及账号、空间标识',
 remote_identity_mismatch:'远端空间身份与原绑定不一致', remote_conflict:'远端目标冲突，请核实后显式接管', management_credential_unavailable:'管理凭据不可用', remote_not_found:'原远端账号不存在', transport_unavailable:'暂时无法连接远端', workspace_unavailable:'工作区暂不可用', management_result_unknown:'管理请求结果未知，禁止直接重复写入', remote_observed_requires_verification:'已查询远端，请核对原请求已经结束', CHECK_FAILED:'连接检查未通过，原配置继续保留',
}
export function workspaceLabel(value:string|undefined){return value ? labels[value]??value : ''}
