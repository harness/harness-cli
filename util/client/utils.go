package client

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
