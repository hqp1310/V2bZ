package panel

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCommonNodeFlexibleDecodeLegacyTypes(t *testing.T) {
	data := []byte(`{
		"protocol": "vless",
		"host": "example.com",
		"listen_ip": null,
		"server_port": "443",
		"routes": [
			{"id": "1", "match": "legacy.example", "action": "block", "action_value": null},
			{"id": 2, "match": ["a.example", "b.example"], "action": "outbound", "action_value": {"type":"field"}}
		],
		"base_config": {
			"panel": "zicboard",
			"node_type": "zicnode",
			"push_interval": "60",
			"pull_interval": 120,
			"device_online_min_traffic": "1024",
			"node_report_min_traffic": 2048
		},
		"tls": "2",
		"tls_settings": {
			"xver": 0,
			"server_names": "example.com",
			"short_ids": "abcd",
			"reject_unknown_sni": true,
			"server_port": 443,
			"cert_mode": null,
			"provider": false,
			"dns_env": 123,
			"self_fallback": "1",
			"cert_file": null,
			"key_file": true
		},
		"network_settings": "{\"acceptProxyProtocol\":true}",
		"network": null,
		"encryption_settings": {
			"mode": 1,
			"ticket": null,
			"server_padding": false,
			"private_key": true
		},
		"zero_rtt_handshake": "1",
		"padding_scheme": "[\"pad-a\",\"pad-b\"]",
		"up_mbps": "100",
		"down_mbps": 200,
		"ignore_client_bandwidth": "true"
	}`)

	var node CommonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("decode CommonNode: %v", err)
	}

	if node.Protocol != "vless" || node.Host != "example.com" || node.ListenIP != "" {
		t.Fatalf("unexpected identity fields: protocol=%q host=%q listen_ip=%q", node.Protocol, node.Host, node.ListenIP)
	}
	if node.ServerPort != 443 || node.Tls != Reality || node.UpMbps != 100 || node.DownMbps != 200 {
		t.Fatalf("unexpected numeric fields: port=%d tls=%d up=%d down=%d", node.ServerPort, node.Tls, node.UpMbps, node.DownMbps)
	}
	if !node.ZeroRTTHandshake || !node.Ignore_Client_Bandwidth {
		t.Fatalf("bool-like fields were not normalized: zero_rtt=%v ignore_bandwidth=%v", node.ZeroRTTHandshake, node.Ignore_Client_Bandwidth)
	}
	if !reflect.DeepEqual(node.PaddingScheme, []string{"pad-a", "pad-b"}) {
		t.Fatalf("unexpected padding scheme: %#v", node.PaddingScheme)
	}
	if string(node.NetworkSettings) != `{"acceptProxyProtocol":true}` {
		t.Fatalf("unexpected network_settings: %s", node.NetworkSettings)
	}

	if node.BaseConfig == nil {
		t.Fatal("base_config was not decoded")
	}
	if node.BaseConfig.Panel != "zicboard" || node.BaseConfig.NodeType != "zicnode" {
		t.Fatalf("unexpected base_config identity: %#v", node.BaseConfig)
	}
	if node.BaseConfig.DeviceOnlineMinTraffic != 1024 || node.BaseConfig.NodeReportMinTraffic != 2048 {
		t.Fatalf("unexpected traffic thresholds: %#v", node.BaseConfig)
	}
	if intervalToTime(node.BaseConfig.PushInterval) != 60*time.Second {
		t.Fatalf("unexpected push interval: %s", intervalToTime(node.BaseConfig.PushInterval))
	}
	if intervalToTime(node.BaseConfig.PullInterval) != 120*time.Second {
		t.Fatalf("unexpected pull interval: %s", intervalToTime(node.BaseConfig.PullInterval))
	}

	settings := node.TlsSettings
	if settings.Xver != 0 {
		t.Fatalf("unexpected xver: %d", settings.Xver)
	}
	if !reflect.DeepEqual(settings.ServerNames, []string{"example.com"}) {
		t.Fatalf("unexpected server names: %#v", settings.ServerNames)
	}
	if !reflect.DeepEqual(settings.ShortIds, []string{"abcd"}) {
		t.Fatalf("unexpected short ids: %#v", settings.ShortIds)
	}
	if settings.RejectUnknownSni != "1" {
		t.Fatalf("unexpected reject_unknown_sni: %q", settings.RejectUnknownSni)
	}
	if settings.ServerPort != "443" || settings.Provider != "false" || settings.DNSEnv != "123" || settings.KeyFile != "true" {
		t.Fatalf("tls string fields were not normalized: %#v", settings)
	}
	if !settings.SelfFallback {
		t.Fatalf("self_fallback was not normalized: %#v", settings)
	}
	if node.EncryptionSettings.Mode != "1" || node.EncryptionSettings.ServerPadding != "false" || node.EncryptionSettings.PrivateKey != "true" {
		t.Fatalf("encryption settings were not normalized: %#v", node.EncryptionSettings)
	}

	if len(node.Routes) != 2 {
		t.Fatalf("expected two routes, got %d", len(node.Routes))
	}
	if !reflect.DeepEqual(node.Routes[0].Match, []string{"legacy.example"}) {
		t.Fatalf("unexpected string route match: %#v", node.Routes[0].Match)
	}
	if !reflect.DeepEqual(node.Routes[1].Match, []string{"a.example", "b.example"}) {
		t.Fatalf("unexpected array route match: %#v", node.Routes[1].Match)
	}
	if node.Routes[1].ActionValue == nil || *node.Routes[1].ActionValue != `{"type":"field"}` {
		t.Fatalf("unexpected object action_value: %#v", node.Routes[1].ActionValue)
	}
}

func TestTlsSettingsFlexibleDecodeVariants(t *testing.T) {
	data := []byte(`{
		"xver": "2",
		"server_names": ["one.example", 2, true, null],
		"short_ids": "[\"sid-a\",\"sid-b\"]",
		"reject_unknown_sni": 1,
		"server_port": "443"
	}`)

	var settings TlsSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode TlsSettings: %v", err)
	}

	if settings.Xver != 2 {
		t.Fatalf("unexpected xver: %d", settings.Xver)
	}
	if !reflect.DeepEqual(settings.ServerNames, []string{"one.example", "2", "true"}) {
		t.Fatalf("unexpected server names: %#v", settings.ServerNames)
	}
	if !reflect.DeepEqual(settings.ShortIds, []string{"sid-a", "sid-b"}) {
		t.Fatalf("unexpected short ids: %#v", settings.ShortIds)
	}
	if settings.RejectUnknownSni != "1" {
		t.Fatalf("unexpected reject_unknown_sni: %q", settings.RejectUnknownSni)
	}
}

func TestWarpSettingsFlexibleDecode(t *testing.T) {
	data := []byte(`{
		"enable": "1",
		"mode": "manual",
		"fail_policy": "block",
		"mtu": "1280",
		"domain_strategy": "ForceIPv4v6",
		"private_key": true,
		"peer_public_key": 123,
		"endpoint": null,
		"addresses": "172.16.0.2/32, 2606:4700:110:8765::2/128",
		"reserved": "1,2,3"
	}`)

	var settings WarpSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode WarpSettings: %v", err)
	}

	if !settings.Enable || settings.Mode != "manual" || settings.FailPolicy != "block" {
		t.Fatalf("unexpected flags: %#v", settings)
	}
	if settings.MTU != 1280 || settings.DomainStrategy != "ForceIPv4v6" || settings.Endpoint != "engage.cloudflareclient.com:2408" {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
	if settings.PrivateKey != "true" || settings.PeerPublicKey != "123" {
		t.Fatalf("string fields were not normalized: %#v", settings)
	}
	if !reflect.DeepEqual(settings.Addresses, []string{"172.16.0.2/32", "2606:4700:110:8765::2/128"}) {
		t.Fatalf("unexpected addresses: %#v", settings.Addresses)
	}
	if !reflect.DeepEqual(settings.Reserved, []byte{1, 2, 3}) {
		t.Fatalf("unexpected reserved: %#v", settings.Reserved)
	}
}

func TestCommonNodeFlexibleDecodeWarpSettings(t *testing.T) {
	data := []byte(`{
		"warp_settings": {
			"enable": 1,
			"mode": "auto",
			"fail_policy": "direct",
			"addresses": ["172.16.0.2/32", "2606:4700:110:8765::2/128"],
			"reserved": [4, 5, 6]
		}
	}`)

	var node CommonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("decode CommonNode: %v", err)
	}
	if !node.WarpSettings.Enable || node.WarpSettings.Mode != "auto" || node.WarpSettings.FailPolicy != "direct" {
		t.Fatalf("unexpected warp settings: %#v", node.WarpSettings)
	}
	if !reflect.DeepEqual(node.WarpSettings.Reserved, []byte{4, 5, 6}) {
		t.Fatalf("unexpected reserved bytes: %#v", node.WarpSettings.Reserved)
	}
}

func TestTlsSettingsFlexibleDecodeXverAndRejectUnknownSniValues(t *testing.T) {
	tests := []struct {
		name             string
		xverJSON         string
		rejectUnknownSNI string
		expectXver       uint64
		expectReject     string
	}{
		{name: "number zero", xverJSON: `0`, rejectUnknownSNI: `false`, expectXver: 0, expectReject: "0"},
		{name: "string zero", xverJSON: `"0"`, rejectUnknownSNI: `"false"`, expectXver: 0, expectReject: "0"},
		{name: "null", xverJSON: `null`, rejectUnknownSNI: `null`, expectXver: 0, expectReject: ""},
		{name: "empty string", xverJSON: `""`, rejectUnknownSNI: `""`, expectXver: 0, expectReject: ""},
		{name: "string number", xverJSON: `"3"`, rejectUnknownSNI: `"true"`, expectXver: 3, expectReject: "1"},
		{name: "numeric true", xverJSON: `1`, rejectUnknownSNI: `1`, expectXver: 1, expectReject: "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{
				"xver": ` + tt.xverJSON + `,
				"reject_unknown_sni": ` + tt.rejectUnknownSNI + `
			}`)

			var settings TlsSettings
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatalf("decode TlsSettings: %v", err)
			}
			if settings.Xver != tt.expectXver {
				t.Fatalf("unexpected xver: got %d want %d", settings.Xver, tt.expectXver)
			}
			if settings.RejectUnknownSni != tt.expectReject {
				t.Fatalf("unexpected reject_unknown_sni: got %q want %q", settings.RejectUnknownSni, tt.expectReject)
			}
		})
	}
}

func TestFlexibleDecodeEmptyValues(t *testing.T) {
	data := []byte(`{
		"tls_settings": {"xver": "", "server_names": "", "short_ids": null, "reject_unknown_sni": 0},
		"padding_scheme": "",
		"zero_rtt_handshake": 0,
		"routes": [{"match": null}]
	}`)

	var node CommonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("decode CommonNode with empty values: %v", err)
	}
	if node.TlsSettings.Xver != 0 || node.TlsSettings.RejectUnknownSni != "0" {
		t.Fatalf("unexpected empty tls settings: %#v", node.TlsSettings)
	}
	if node.PaddingScheme != nil || node.ZeroRTTHandshake {
		t.Fatalf("unexpected empty top-level fields: padding=%#v zero_rtt=%v", node.PaddingScheme, node.ZeroRTTHandshake)
	}
	if len(node.Routes) != 1 || node.Routes[0].Match != nil {
		t.Fatalf("unexpected empty route match: %#v", node.Routes)
	}
}

func TestCommonNodeCertTargetPrefersSNIOverHost(t *testing.T) {
	node := CommonNode{
		Host: "my-node.example.com",
		TlsSettings: TlsSettings{
			ServerName: "www.apple.com",
		},
	}
	if got := node.CertTarget(); got != "www.apple.com" {
		t.Fatalf("CertTarget got %q, want server_name", got)
	}

	node = CommonNode{
		Host: "fallback.example.com",
	}
	if got := node.CertTarget(); got != "fallback.example.com" {
		t.Fatalf("CertTarget fallback got %q, want host", got)
	}

	node = CommonNode{
		Host: "my-node.example.com",
		TlsSettings: TlsSettings{
			ServerName:  "b.example.com",
			ServerNames: []string{"a.example.com"},
		},
	}
	if got := node.CertTarget(); got != "a.example.com" {
		t.Fatalf("CertTarget server_names got %q, want first server_names entry", got)
	}
}

func TestDefaultACMEEmail(t *testing.T) {
	if defaultACMEEmail != "contact@zicnet.vn" {
		t.Fatalf("defaultACMEEmail got %q, want contact@zicnet.vn", defaultACMEEmail)
	}
}

func TestCommonNodeEffectiveSecurityUsesImplicitTLSProtocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		tls      int
		want     int
	}{
		{name: "trojan old zero", protocol: "trojan", tls: None, want: Tls},
		{name: "hysteria2 old zero", protocol: "hysteria2", tls: None, want: Tls},
		{name: "tuic old zero", protocol: "tuic", tls: None, want: Tls},
		{name: "anytls old zero", protocol: "anytls", tls: None, want: Tls},
		{name: "anytls reality", protocol: "anytls", tls: Reality, want: Reality},
		{name: "trojan bad reality", protocol: "trojan", tls: Reality, want: Tls},
		{name: "vmess bad reality", protocol: "vmess", tls: Reality, want: Tls},
		{name: "vless none", protocol: "vless", tls: None, want: None},
		{name: "vless tls", protocol: "vless", tls: Tls, want: Tls},
		{name: "vless reality", protocol: "vless", tls: Reality, want: Reality},
		{name: "shadowsocks ignores tls", protocol: "shadowsocks", tls: Tls, want: None},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := CommonNode{Protocol: tt.protocol, Tls: tt.tls}
			if got := node.EffectiveSecurity(); got != tt.want {
				t.Fatalf("EffectiveSecurity got %d, want %d", got, tt.want)
			}
		})
	}
}
