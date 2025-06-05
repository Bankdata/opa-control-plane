package global.systemtypes["entitlements:1.0"].library.sample.styra

role_bindings := {
	"SystemPolicyEditor": {"subjects": {
		"ids": ["alice", "bob", "platform-team"],
		"membership-attributes": {"is_admin": true},
	}},
	"WorkspaceAdmin": {"subjects": {"ids": ["admin-team"]}},
	"DenySystemConfigModification": {"subjects": {"ids": ["bob"]}},
}
