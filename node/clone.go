package node

import panel "github.com/ZicBoard/ZicNode/api/zicboard"

func cloneNodeInfo(info *panel.NodeInfo) *panel.NodeInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	cloned.Common = cloneCommonNode(info.Common)
	return &cloned
}

func cloneCommonNode(common *panel.CommonNode) *panel.CommonNode {
	if common == nil {
		return nil
	}
	cloned := *common
	cloned.Routes = cloneRoutes(common.Routes)
	cloned.NetworkSettings = append([]byte(nil), common.NetworkSettings...)
	cloned.PaddingScheme = append([]string(nil), common.PaddingScheme...)
	cloned.BaseConfig = cloneBaseConfig(common.BaseConfig)
	cloned.TlsSettings.ServerNames = append([]string(nil), common.TlsSettings.ServerNames...)
	cloned.TlsSettings.ShortIds = append([]string(nil), common.TlsSettings.ShortIds...)
	cloned.CertInfo = cloneCertInfo(common.CertInfo)
	cloned.WarpSettings.Addresses = append([]string(nil), common.WarpSettings.Addresses...)
	cloned.WarpSettings.Reserved = append([]byte(nil), common.WarpSettings.Reserved...)
	return &cloned
}

func cloneRoutes(routes []panel.Route) []panel.Route {
	if routes == nil {
		return nil
	}
	cloned := make([]panel.Route, len(routes))
	for i := range routes {
		cloned[i] = routes[i]
		cloned[i].Match = append([]string(nil), routes[i].Match...)
		if routes[i].ActionValue != nil {
			value := *routes[i].ActionValue
			cloned[i].ActionValue = &value
		}
	}
	return cloned
}

func cloneBaseConfig(base *panel.BaseConfig) *panel.BaseConfig {
	if base == nil {
		return nil
	}
	cloned := *base
	return &cloned
}

func cloneCertInfo(cert *panel.CertInfo) *panel.CertInfo {
	if cert == nil {
		return nil
	}
	cloned := *cert
	if cert.DNSEnv != nil {
		cloned.DNSEnv = make(map[string]string, len(cert.DNSEnv))
		for key, value := range cert.DNSEnv {
			cloned.DNSEnv[key] = value
		}
	}
	return &cloned
}
