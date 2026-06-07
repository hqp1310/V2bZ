package core

import (
	"encoding/json"
	"net"
	"strings"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/app/dns"
	"github.com/xtls/xray-core/app/router"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	coreConf "github.com/xtls/xray-core/infra/conf"
)

// hasPublicIPv6 checks if the machine has a public IPv6 address
func hasPublicIPv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		// Check if it's IPv6, not loopback, not link-local, not private/ULA
		if ip.To4() == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsPrivate() {
			return true
		}
	}
	return false
}

func hasOutboundWithTag(list []*core.OutboundHandlerConfig, tag string) bool {
	for _, o := range list {
		if o != nil && o.Tag == tag {
			return true
		}
	}
	return false
}

func parseCustomRouteOutbound(actionValue *string) (*coreConf.OutboundDetourConfig, bool, bool, error) {
	cleaned, removedAllowInsecure, hasPinnedPeerCertSha256, err := sanitizeCustomRouteOutbound(actionValue)
	if err != nil {
		return nil, false, false, err
	}
	outbound := &coreConf.OutboundDetourConfig{}
	if err := json.Unmarshal(cleaned, outbound); err != nil {
		return nil, false, false, err
	}
	return outbound, removedAllowInsecure, hasPinnedPeerCertSha256, nil
}

func sanitizeCustomRouteOutbound(actionValue *string) ([]byte, bool, bool, error) {
	if actionValue == nil {
		return nil, false, false, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(*actionValue), &raw); err != nil {
		return nil, false, false, err
	}

	streamSettings, ok := raw["streamSettings"].(map[string]interface{})
	if !ok {
		return []byte(*actionValue), false, false, nil
	}

	removedAllowInsecure := false
	hasPinnedPeerCertSha256 := false
	for key, value := range streamSettings {
		if !strings.HasSuffix(key, "Settings") {
			continue
		}
		settings, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if pinned, ok := settings["pinnedPeerCertSha256"].(string); ok && strings.TrimSpace(pinned) != "" {
			hasPinnedPeerCertSha256 = true
		}
		if _, ok := settings["allowInsecure"]; ok {
			delete(settings, "allowInsecure")
			removedAllowInsecure = true
		}
	}

	if !removedAllowInsecure {
		return []byte(*actionValue), false, hasPinnedPeerCertSha256, nil
	}

	cleaned, err := json.Marshal(raw)
	if err != nil {
		return nil, false, false, err
	}
	return cleaned, true, hasPinnedPeerCertSha256, nil
}

func logRemovedCustomOutboundAllowInsecure(info *panel.NodeInfo, route panel.Route, outbound *coreConf.OutboundDetourConfig, hasPinnedPeerCertSha256 bool) {
	log.WithFields(log.Fields{
		"inbound_tag":             info.Tag,
		"route_action":            route.Action,
		"outbound_tag":            outbound.Tag,
		"pinned_peer_cert_sha256": hasPinnedPeerCertSha256,
	}).Warn("removed deprecated allowInsecure from custom route outbound TLS settings")
}

func appendCatchAllOutboundRule(routerConfig *coreConf.RouterConfig, inboundTag, outboundTag string) {
	rule := map[string]interface{}{
		"inboundTag":  inboundTag,
		"network":     "tcp,udp",
		"outboundTag": outboundTag,
	}
	rawRule, err := json.Marshal(rule)
	if err != nil {
		return
	}
	routerConfig.RuleList = append(routerConfig.RuleList, rawRule)
}

func appendDefaultDNSRule(routerConfig *coreConf.RouterConfig, inboundTags []string) {
	rule := map[string]interface{}{
		"port":        "53",
		"network":     "udp",
		"outboundTag": "dns_out",
	}
	if len(inboundTags) > 0 {
		rule["inboundTag"] = inboundTags
	}
	rawRule, err := json.Marshal(rule)
	if err != nil {
		return
	}
	routerConfig.RuleList = append(routerConfig.RuleList, rawRule)
}

func nonWarpInboundTags(infos []*panel.NodeInfo) ([]string, bool) {
	nonWarpTags := make([]string, 0, len(infos))
	hasWarp := false
	for _, info := range infos {
		if info == nil || info.Common == nil {
			continue
		}
		if warpEnabled(info) {
			hasWarp = true
			continue
		}
		if strings.TrimSpace(info.Tag) != "" {
			nonWarpTags = append(nonWarpTags, info.Tag)
		}
	}
	return nonWarpTags, hasWarp
}

func GetCustomConfig(infos []*panel.NodeInfo) (*dns.Config, []*core.OutboundHandlerConfig, *router.Config, error) {
	//dns
	queryStrategy := "UseIPv4v6"
	if !hasPublicIPv6() {
		queryStrategy = "UseIPv4"
	}
	coreDnsConfig := &coreConf.DNSConfig{
		Servers: []*coreConf.NameServerConfig{
			{
				Address: &coreConf.Address{
					Address: xnet.ParseAddress("localhost"),
				},
			},
		},
		QueryStrategy: queryStrategy,
	}
	//outbound
	defaultoutbound, _ := buildDefaultOutbound()
	coreOutboundConfig := append([]*core.OutboundHandlerConfig{}, defaultoutbound)
	block, _ := buildBlockOutbound()
	coreOutboundConfig = append(coreOutboundConfig, block)
	dns, _ := buildDnsOutbound()
	coreOutboundConfig = append(coreOutboundConfig, dns)

	//route
	domainStrategy := "AsIs"
	coreRouterConfig := &coreConf.RouterConfig{
		RuleList:       []json.RawMessage{},
		DomainStrategy: &domainStrategy,
	}
	nonWarpTags, hasWarp := nonWarpInboundTags(infos)
	if !hasWarp {
		appendDefaultDNSRule(coreRouterConfig, nil)
	} else if len(nonWarpTags) > 0 {
		appendDefaultDNSRule(coreRouterConfig, nonWarpTags)
	}

	for _, info := range infos {
		if info == nil || info.Common == nil {
			continue
		}
		useWarp := warpEnabled(info)
		for _, route := range info.Common.Routes {
			switch route.Action {
			case "dns":
				if route.ActionValue == nil {
					continue
				}
				server := &coreConf.NameServerConfig{
					Address: &coreConf.Address{
						Address: xnet.ParseAddress(*route.ActionValue),
					},
				}
				if len(route.Match) != 0 {
					server.Domains = route.Match
					server.SkipFallback = true
				}
				coreDnsConfig.Servers = append(coreDnsConfig.Servers, server)
			case "block":
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"domain":      route.Match,
					"outboundTag": "block",
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
			case "block_ip":
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"ip":          route.Match,
					"outboundTag": "block",
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
			case "block_port":
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"port":        strings.Join(route.Match, ","),
					"outboundTag": "block",
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
			case "protocol":
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"protocol":    route.Match,
					"outboundTag": "block",
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
			case "route":
				if route.ActionValue == nil {
					continue
				}
				outbound, removedAllowInsecure, hasPinnedPeerCertSha256, err := parseCustomRouteOutbound(route.ActionValue)
				if err != nil {
					continue
				}
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"domain":      route.Match,
					"outboundTag": outbound.Tag,
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
				if hasOutboundWithTag(coreOutboundConfig, outbound.Tag) {
					continue
				}
				if removedAllowInsecure {
					logRemovedCustomOutboundAllowInsecure(info, route, outbound, hasPinnedPeerCertSha256)
				}
				custom_outbound, err := outbound.Build()
				if err != nil {
					continue
				}
				coreOutboundConfig = append(coreOutboundConfig, custom_outbound)
			case "route_ip":
				if route.ActionValue == nil {
					continue
				}
				outbound, removedAllowInsecure, hasPinnedPeerCertSha256, err := parseCustomRouteOutbound(route.ActionValue)
				if err != nil {
					continue
				}
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"ip":          route.Match,
					"outboundTag": outbound.Tag,
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
				if hasOutboundWithTag(coreOutboundConfig, outbound.Tag) {
					continue
				}
				if removedAllowInsecure {
					logRemovedCustomOutboundAllowInsecure(info, route, outbound, hasPinnedPeerCertSha256)
				}
				custom_outbound, err := outbound.Build()
				if err != nil {
					continue
				}
				coreOutboundConfig = append(coreOutboundConfig, custom_outbound)
			case "default_out":
				if useWarp {
					log.WithFields(log.Fields{
						"inbound_tag": info.Tag,
						"route_id":    route.Id,
					}).Warn("ignored custom default_out route because WARP outbound is enabled for this node")
					continue
				}
				if route.ActionValue == nil {
					continue
				}
				outbound, removedAllowInsecure, hasPinnedPeerCertSha256, err := parseCustomRouteOutbound(route.ActionValue)
				if err != nil {
					continue
				}
				rule := map[string]interface{}{
					"inboundTag":  info.Tag,
					"network":     "tcp,udp",
					"outboundTag": outbound.Tag,
				}
				rawRule, err := json.Marshal(rule)
				if err != nil {
					continue
				}
				coreRouterConfig.RuleList = append(coreRouterConfig.RuleList, rawRule)
				if hasOutboundWithTag(coreOutboundConfig, outbound.Tag) {
					continue
				}
				if removedAllowInsecure {
					logRemovedCustomOutboundAllowInsecure(info, route, outbound, hasPinnedPeerCertSha256)
				}
				custom_outbound, err := outbound.Build()
				if err != nil {
					continue
				}
				coreOutboundConfig = append(coreOutboundConfig, custom_outbound)
			default:
				continue
			}
		}
		if useWarp {
			warpOutbound, warpTag, err := buildWarpOutbound(info)
			if err != nil {
				fields := log.Fields{
					"inbound_tag": info.Tag,
					"fail_policy": warpFailPolicy(info),
					"error":       err,
				}
				if warpFailPolicy(info) == "block" {
					appendCatchAllOutboundRule(coreRouterConfig, info.Tag, "block")
					log.WithFields(fields).Warn("WARP outbound failed; node traffic will be blocked by fail_policy")
				} else {
					log.WithFields(fields).Warn("WARP outbound failed; node traffic will use direct fallback")
				}
				continue
			}
			if warpOutbound != nil && !hasOutboundWithTag(coreOutboundConfig, warpTag) {
				coreOutboundConfig = append(coreOutboundConfig, warpOutbound)
			}
			appendCatchAllOutboundRule(coreRouterConfig, info.Tag, warpTag)
		}
	}
	DnsConfig, err := coreDnsConfig.Build()
	if err != nil {
		return nil, nil, nil, err
	}
	RouterConfig, err := coreRouterConfig.Build()
	if err != nil {
		return nil, nil, nil, err
	}
	return DnsConfig, coreOutboundConfig, RouterConfig, nil
}
