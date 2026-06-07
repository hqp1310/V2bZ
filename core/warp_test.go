package core

import (
	"testing"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	_ "github.com/ZicBoard/ZicNode/core/distro/all"
)

const (
	testWarpPrivateKey = "uJv5tZMDltsiYEn+kUwb0Ll/CXWhMkaSCWWhfPEZM3A="
	testWarpPeerKey    = "6e65ce0be17517110c17d77288ad87e7fd5252dcc7d09b95a39d61db03df832a"
)

func TestBuildWarpOutboundManual(t *testing.T) {
	info := testWarpNode(panel.WarpSettings{
		Enable:         true,
		Mode:           "manual",
		PrivateKey:     testWarpPrivateKey,
		PeerPublicKey:  testWarpPeerKey,
		Endpoint:       "127.0.0.1:2408",
		Addresses:      []string{"10.1.1.1/32", "fd59:7153:2388:b5fd::1/128"},
		Reserved:       []byte{1, 2, 3},
		MTU:            1280,
		DomainStrategy: "ForceIPv4v6",
	})

	outbound, tag, err := buildWarpOutbound(info)
	if err != nil {
		t.Fatalf("build WARP outbound: %v", err)
	}
	if tag != "warp-node-1" || outbound == nil || outbound.Tag != tag {
		t.Fatalf("unexpected outbound: tag=%q outbound=%#v", tag, outbound)
	}
}

func TestGetCustomConfigAddsWarpCatchAllAfterExplicitRoutes(t *testing.T) {
	info := testWarpNode(testManualWarpSettings())
	info.Common.Routes = []panel.Route{{Id: 1, Match: []string{"blocked.example"}, Action: "block"}}

	_, outbounds, routerConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig: %v", err)
	}
	if !hasOutboundWithTag(outbounds, "warp-node-1") {
		t.Fatalf("expected WARP outbound, got %#v", outbounds)
	}
	if len(routerConfig.Rule) < 2 {
		t.Fatalf("expected block and WARP rules, got %d", len(routerConfig.Rule))
	}
	if routerConfig.Rule[0].GetTag() != "block" {
		t.Fatalf("explicit block route should stay before WARP, got %q", routerConfig.Rule[0].GetTag())
	}
	last := routerConfig.Rule[len(routerConfig.Rule)-1]
	if last.GetTag() != "warp-node-1" || len(last.GetInboundTag()) != 1 || last.GetInboundTag()[0] != "node-1" {
		t.Fatalf("unexpected WARP catch-all rule: %#v", last)
	}
}

func TestGetCustomConfigDefaultDNSRuleSkipsWarpInbound(t *testing.T) {
	warpInfo := testWarpNode(testManualWarpSettings())
	nonWarpInfo := testWarpNode(panel.WarpSettings{})
	nonWarpInfo.Id = 2
	nonWarpInfo.Tag = "node-2"

	_, _, routerConfig, err := GetCustomConfig([]*panel.NodeInfo{warpInfo, nonWarpInfo})
	if err != nil {
		t.Fatalf("GetCustomConfig: %v", err)
	}
	if len(routerConfig.Rule) < 2 {
		t.Fatalf("expected DNS and WARP rules, got %d", len(routerConfig.Rule))
	}
	dnsRule := routerConfig.Rule[0]
	if dnsRule.GetTag() != "dns_out" {
		t.Fatalf("expected DNS rule first for non-WARP nodes, got %#v", dnsRule)
	}
	if len(dnsRule.GetInboundTag()) != 1 || dnsRule.GetInboundTag()[0] != "node-2" {
		t.Fatalf("DNS rule should only target non-WARP inbound tags, got %#v", dnsRule.GetInboundTag())
	}
	last := routerConfig.Rule[len(routerConfig.Rule)-1]
	if last.GetTag() != "warp-node-1" || len(last.GetInboundTag()) != 1 || last.GetInboundTag()[0] != "node-1" {
		t.Fatalf("unexpected WARP catch-all rule: %#v", last)
	}
}

func TestGetCustomConfigWarpFailBlockDoesNotAddDirectCatchAll(t *testing.T) {
	info := testWarpNode(panel.WarpSettings{Enable: true, Mode: "manual", FailPolicy: "block"})

	_, outbounds, routerConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig: %v", err)
	}
	if hasOutboundWithTag(outbounds, "warp-node-1") {
		t.Fatalf("WARP outbound should not be added when manual config is incomplete")
	}
	last := routerConfig.Rule[len(routerConfig.Rule)-1]
	if last.GetTag() != "block" || len(last.GetInboundTag()) != 1 || last.GetInboundTag()[0] != "node-1" {
		t.Fatalf("expected block catch-all, got %#v", last)
	}
}

func TestGetCustomConfigIgnoresDefaultOutWhenWarpEnabled(t *testing.T) {
	actionValue := `{"protocol":"freedom","tag":"custom-default","settings":{"domainStrategy":"UseIPv4"}}`
	info := testWarpNode(testManualWarpSettings())
	info.Common.Routes = []panel.Route{{Id: 7, Action: "default_out", ActionValue: &actionValue}}

	_, outbounds, routerConfig, err := GetCustomConfig([]*panel.NodeInfo{info})
	if err != nil {
		t.Fatalf("GetCustomConfig: %v", err)
	}
	if hasOutboundWithTag(outbounds, "custom-default") {
		t.Fatalf("default_out should be ignored when WARP is enabled")
	}
	last := routerConfig.Rule[len(routerConfig.Rule)-1]
	if last.GetTag() != "warp-node-1" {
		t.Fatalf("expected WARP catch-all, got %#v", last)
	}
}

func TestWarpSidecarPathUsesPanelAndNodeID(t *testing.T) {
	first := warpSidecarPath(&panel.NodeInfo{Id: 9, Tag: "[https://panel.example]-vless:9"})
	second := warpSidecarPath(&panel.NodeInfo{Id: 9, Tag: "[https://panel.example]-trojan:9"})
	third := warpSidecarPath(&panel.NodeInfo{Id: 10, Tag: "[https://panel.example]-vless:10"})

	if first != second {
		t.Fatalf("same panel/node should reuse WARP sidecar across protocol changes: %q != %q", first, second)
	}
	if first == third {
		t.Fatalf("different node IDs should not share WARP sidecar: %q", first)
	}
}

func testManualWarpSettings() panel.WarpSettings {
	return panel.WarpSettings{
		Enable:         true,
		Mode:           "manual",
		FailPolicy:     "direct",
		PrivateKey:     testWarpPrivateKey,
		PeerPublicKey:  testWarpPeerKey,
		Endpoint:       "127.0.0.1:2408",
		Addresses:      []string{"10.1.1.1/32", "fd59:7153:2388:b5fd::1/128"},
		MTU:            1280,
		DomainStrategy: "ForceIPv4v6",
	}
}

func testWarpNode(settings panel.WarpSettings) *panel.NodeInfo {
	return &panel.NodeInfo{
		Id:  1,
		Tag: "node-1",
		Common: &panel.CommonNode{
			Protocol:     "vless",
			WarpSettings: settings,
		},
	}
}
