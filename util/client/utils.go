package client

import "harness/config"

func GetScopeRef() string {
	return GetRef(config.Global.AccountID, config.Global.OrgID, config.Global.ProjectID)
}

func GetRef(params ...string) string {
	ref := ""
	for _, param := range params {
		if param != "" {
			ref += param + "/"
		}
	}
	if len(ref) > 0 {
		ref = ref[:len(ref)-1]
	}
	return ref
}
