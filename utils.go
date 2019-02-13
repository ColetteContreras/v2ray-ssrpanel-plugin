package v2ray_ssrpanel_plugin

func InStr(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
