package global.systemtypes["entitlements:1.0"].library.sample.car_info_store

input_request := {
	"action": "create",
	"resource": "resource-id",
	"subject": "subject-id",
}

input_request_more_fields := {
	"action": "CREATE",
	"context": {
		"channel": "API-MOBILE-BROWSER",
		"location": "someplace",
	},
	"groups": [
		"group1",
		"group2",
	],
	"jwt": "jwt token",
	"resource": "aResource:action",
	"resource-attributes": {
		"attr1": "value1",
		"attr2": "value2",
	},
	"roles": [
		"aRole1",
		"aRole2",
	],
	"subject": "aSubject",
	"subject-attributes": {
		"attr1": "value1",
		"attr2": "value2",
	},
}
