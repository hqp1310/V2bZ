package core

import (
	"fmt"
	"sync"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	"github.com/ZicBoard/ZicNode/common/reload"
	"github.com/ZicBoard/ZicNode/conf"
	"github.com/ZicBoard/ZicNode/core/app/dispatcher"
	_ "github.com/ZicBoard/ZicNode/core/distro/all"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/routing"
	coreConf "github.com/xtls/xray-core/infra/conf"
	"google.golang.org/protobuf/proto"
)

type AddUsersParams struct {
	Tag   string
	Users []panel.UserInfo
	*panel.NodeInfo
}

type V2Core struct {
	Config     *conf.Conf
	ReloadCh   chan reload.Event
	access     sync.Mutex
	Server     *core.Instance
	users      *UserMap
	ihm        inbound.Manager
	ohm        outbound.Manager
	dispatcher *dispatcher.DefaultDispatcher
}

type UserMap struct {
	uidMap  map[string]int
	mapLock sync.RWMutex
}

func New(config *conf.Conf) *V2Core {
	core := &V2Core{
		Config: config,
		users: &UserMap{
			uidMap: make(map[string]int),
		},
	}
	return core
}

func (v *V2Core) Start(infos []*panel.NodeInfo) error {
	if err := v.Prepare(infos); err != nil {
		return err
	}
	return v.StartPrepared()
}

func (v *V2Core) Prepare(infos []*panel.NodeInfo) error {
	v.access.Lock()
	defer v.access.Unlock()
	server, err := getCore(v.Config, infos)
	if err != nil {
		return err
	}
	v.Server = server
	return nil
}

func (v *V2Core) StartPrepared() error {
	v.access.Lock()
	defer v.access.Unlock()
	if v.Server == nil {
		return fmt.Errorf("core has not been prepared")
	}
	if err := v.Server.Start(); err != nil {
		return err
	}
	ihm, ok := v.Server.GetFeature(inbound.ManagerType()).(inbound.Manager)
	if !ok {
		return fmt.Errorf("core inbound manager is unavailable")
	}
	ohm, ok := v.Server.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok {
		return fmt.Errorf("core outbound manager is unavailable")
	}
	d, ok := v.Server.GetFeature(routing.DispatcherType()).(*dispatcher.DefaultDispatcher)
	if !ok {
		return fmt.Errorf("core dispatcher is unavailable")
	}
	v.ihm = ihm
	v.ohm = ohm
	v.dispatcher = d
	return nil
}

func (v *V2Core) ActiveLinkCount() int {
	v.access.Lock()
	defer v.access.Unlock()
	if v.dispatcher == nil {
		return 0
	}
	return v.dispatcher.ActiveLinkCount()
}

func (v *V2Core) Close() error {
	return v.CloseWithReason("", nil)
}

func (v *V2Core) CloseWithReason(reason string, fields log.Fields) error {
	v.access.Lock()
	defer v.access.Unlock()
	if v.dispatcher != nil && reason != "" {
		v.dispatcher.CloseAllManagedLinks(reason, fields)
	}
	v.Config = nil
	v.ihm = nil
	v.ohm = nil
	v.dispatcher = nil
	if v.Server == nil {
		return nil
	}
	err := v.Server.Close()
	if err != nil {
		return err
	}
	v.Server = nil
	return nil
}

func getCore(c *conf.Conf, infos []*panel.NodeInfo) (*core.Instance, error) {
	// Log Config
	coreLogConfig := &coreConf.LogConfig{
		LogLevel:  c.LogConfig.Level,
		AccessLog: c.LogConfig.Access,
		ErrorLog:  c.LogConfig.Output,
	}
	// Custom config
	dnsConfig, outBoundConfig, routeConfig, err := GetCustomConfig(infos)
	if err != nil {
		return nil, fmt.Errorf("failed to build custom config: %w", err)
	}
	// Inbound config
	var inBoundConfig []*core.InboundHandlerConfig

	// Policy config
	levelPolicyConfig := &coreConf.Policy{
		StatsUserUplink:   true,
		StatsUserDownlink: true,
		Handshake:         proto.Uint32(4),
		ConnectionIdle:    proto.Uint32(120),
		UplinkOnly:        proto.Uint32(2),
		DownlinkOnly:      proto.Uint32(4),
		BufferSize:        proto.Int32(128),
	}
	corePolicyConfig := &coreConf.PolicyConfig{}
	corePolicyConfig.Levels = map[uint32]*coreConf.Policy{0: levelPolicyConfig}
	policyConfig, _ := corePolicyConfig.Build()
	// Build Xray conf
	config := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(coreLogConfig.Build()),
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&stats.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(policyConfig),
			serial.ToTypedMessage(dnsConfig),
			serial.ToTypedMessage(routeConfig),
		},
		Inbound:  inBoundConfig,
		Outbound: outBoundConfig,
	}
	server, err := core.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}
	log.Info("Xray Core Version: ", core.Version())
	return server, nil
}
