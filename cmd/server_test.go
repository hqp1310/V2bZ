package cmd

import (
	"testing"
	"time"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	reloadEvent "github.com/ZicBoard/ZicNode/common/reload"
	"github.com/ZicBoard/ZicNode/conf"
)

func TestRuntimeSignatureIgnoresLogAndRuntimeOnlyBaseConfig(t *testing.T) {
	baseConfig := testRuntimeConf()
	baseInfos := []*panel.NodeInfo{testRuntimeNodeInfo()}
	baseSignature := buildRuntimeSignature(baseConfig, baseInfos)

	logOnly := cloneConfig(baseConfig)
	logOnly.LogConfig.Level = "debug"
	logOnly.LogConfig.Output = "/tmp/zicnode.log"
	logOnly.LogConfig.Access = "/tmp/access.log"
	if got := buildRuntimeSignature(logOnly, baseInfos); got != baseSignature {
		t.Fatalf("log-only config changed runtime signature: got %q want %q", got, baseSignature)
	}

	runtimeOnlyInfo := testRuntimeNodeInfo()
	runtimeOnlyInfo.Common.BaseConfig.PushInterval = 10
	runtimeOnlyInfo.Common.BaseConfig.PullInterval = "20"
	runtimeOnlyInfo.Common.BaseConfig.DeviceOnlineMinTraffic = 4096
	runtimeOnlyInfo.Common.BaseConfig.NodeReportMinTraffic = 8192
	if got := buildRuntimeSignature(baseConfig, []*panel.NodeInfo{runtimeOnlyInfo}); got != baseSignature {
		t.Fatalf("runtime-only base_config changed runtime signature: got %q want %q", got, baseSignature)
	}
}

func TestRuntimeSignatureChangesForLocalAndCoreConfig(t *testing.T) {
	baseConfig := testRuntimeConf()
	baseInfos := []*panel.NodeInfo{testRuntimeNodeInfo()}
	baseSignature := buildRuntimeSignature(baseConfig, baseInfos)

	localChanged := cloneConfig(baseConfig)
	localChanged.NodeConfigs[0].Timeout++
	if got := buildRuntimeSignature(localChanged, baseInfos); got == baseSignature {
		t.Fatal("local node timeout change did not change runtime signature")
	}

	coreChanged := testRuntimeNodeInfo()
	coreChanged.Common.ServerPort = 8443
	if got := buildRuntimeSignature(baseConfig, []*panel.NodeInfo{coreChanged}); got == baseSignature {
		t.Fatal("core server port change did not change runtime signature")
	}
}

func TestSameLocalRuntimeConfigNormalizesDefaults(t *testing.T) {
	withDefaults := testRuntimeConf()
	withoutDefaults := testRuntimeConf()
	withoutDefaults.NodeConfigs[0].Timeout = 0
	withoutDefaults.NodeConfigs[0].RetryCount = nil

	if !sameLocalRuntimeConfig(withDefaults, withoutDefaults) {
		t.Fatal("effective default timeout/retry count should not change local runtime config")
	}
}

func TestIsFatalReloadFailure(t *testing.T) {
	if !isFatalReloadFailure(reloadEvent.ReasonConfigFileChanged, true) {
		t.Fatal("local runtime config change should be fatal on reload failure")
	}
	if !isFatalReloadFailure(reloadEvent.ReasonNodeConfigChanged, false) {
		t.Fatal("node config changed reload failure should be fatal")
	}
	if isFatalReloadFailure(reloadEvent.ReasonConfigFileChanged, false) {
		t.Fatal("unchanged config file reload failure should keep old runtime")
	}
}

func testRuntimeConf() *conf.Conf {
	retryCount := conf.DefaultNodeRetryCount
	return &conf.Conf{
		LogConfig: conf.LogConfig{Level: "info"},
		NodeConfigs: []conf.NodeConfig{
			{
				APIHost:    "https://panel.example.com",
				NodeID:     1,
				Key:        "secret",
				Timeout:    conf.DefaultNodeTimeout,
				RetryCount: &retryCount,
			},
		},
	}
}

func testRuntimeNodeInfo() *panel.NodeInfo {
	return &panel.NodeInfo{
		Id:           1,
		Type:         "vless",
		Security:     panel.Tls,
		PushInterval: time.Minute,
		PullInterval: 2 * time.Minute,
		Tag:          "vless-1",
		Common: &panel.CommonNode{
			Protocol:   "vless",
			Host:       "example.com",
			ListenIP:   "0.0.0.0",
			ServerPort: 443,
			Routes: []panel.Route{
				{Id: 1, Match: []string{"example.com"}, Action: "direct"},
			},
			BaseConfig: &panel.BaseConfig{
				Panel:                  "zicboard",
				NodeType:               "zicnode",
				PushInterval:           60,
				PullInterval:           120,
				DeviceOnlineMinTraffic: 1024,
				NodeReportMinTraffic:   2048,
			},
			Tls: panel.Tls,
			CertInfo: &panel.CertInfo{
				CertMode: "file",
				CertFile: "/tmp/cert.pem",
				KeyFile:  "/tmp/key.pem",
			},
		},
	}
}
