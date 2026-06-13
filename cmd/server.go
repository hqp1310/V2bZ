package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	reloadEvent "github.com/ZicBoard/ZicNode/common/reload"
	"github.com/ZicBoard/ZicNode/conf"
	"github.com/ZicBoard/ZicNode/core"
	"github.com/ZicBoard/ZicNode/limiter"
	"github.com/ZicBoard/ZicNode/node"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	config string
	watch  bool
)

var serverCommand = cobra.Command{
	Use:   "server",
	Short: "Run ZicNode server",
	Run:   serverHandle,
	Args:  cobra.NoArgs,
}

func init() {
	serverCommand.PersistentFlags().
		StringVarP(&config, "config", "c",
			"/etc/zicnode/config.json", "config file path")
	serverCommand.PersistentFlags().
		BoolVarP(&watch, "watch", "w",
			true, "watch file path change")
	command.AddCommand(&serverCommand)
}

func serverHandle(_ *cobra.Command, _ []string) {
	showVersion()
	c := conf.New()
	err := c.LoadFromPath(config)
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: true,
		DisableQuote:     true,
		PadLevelText:     false,
	})
	if err != nil {
		log.WithField("err", err).Error("Load config file failed")
		return
	}
	applyLogConfig(c.LogConfig)
	// Enable pprof if configured
	if c.PprofPort != 0 {
		go func() {
			log.Infof("Starting pprof server on :%d", c.PprofPort)
			if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", c.PprofPort), nil); err != nil {
				log.WithField("err", err).Error("pprof server failed")
			}
		}()
	}
	// init limiter
	limiter.Init()
	reloadCh := make(chan reloadEvent.Event, 1)
	current, err := newPreparedRuntime(c, reloadCh)
	if err != nil {
		log.WithField("err", err).Error("prepare runtime failed")
		return
	}
	if err := current.start(); err != nil {
		_ = current.close("startup_failed", log.Fields{"reason": "startup_failed"})
		log.WithField("err", err).Error("start runtime failed")
		return
	}
	defer func() {
		_ = current.close("shutdown", log.Fields{"reason": "shutdown"})
	}()
	log.Info("Nodes started")
	if watch {
		// On file change, just signal reload; do not run reload concurrently here
		err = c.Watch(config, func() {
			select {
			case reloadCh <- reloadEvent.Event{Reason: reloadEvent.ReasonConfigFileChanged}:
			default: // drop if a reload is already queued
			}
		})
		if err != nil {
			log.WithField("err", err).Error("start watch failed")
			return
		}
	}
	// clear memory
	runtime.GC()

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-osSignals:
			log.Info("received shutdown signal, exiting")
			return
		case ev := <-reloadCh:
			startedAt := time.Now()
			fields := reloadEventFields(ev)
			fields["event"] = "zicnode_disconnect"
			fields["action"] = "reload_begin"
			fields["active_links"] = current.ActiveLinkCount()
			log.WithFields(fields).Error("reload begin, active links will disconnect")
			outcome, err := reload(config, &current, ev)
			if err != nil {
				failedFields := reloadEventFields(ev)
				failedFields["event"] = "zicnode_reload"
				failedFields["result"] = "failed"
				failedFields["duration_ms"] = time.Since(startedAt).Milliseconds()
				failedFields["err"] = err
				if failure, ok := err.(*reloadFailure); ok {
					failedFields["action"] = failure.action
					failedFields["fatal"] = failure.fatal
				}
				log.WithFields(failedFields).Error("reload failed")
				if failure, ok := err.(*reloadFailure); ok && failure.fatal {
					os.Exit(1)
				}
				continue
			}
			successFields := reloadEventFields(ev)
			successFields["event"] = "zicnode_reload"
			successFields["result"] = "success"
			if outcome != nil {
				successFields["result"] = outcome.result
				successFields["action"] = outcome.action
			}
			successFields["duration_ms"] = time.Since(startedAt).Milliseconds()
			log.WithFields(successFields).Error("reload completed")
		}
	}
}

type runtimeBundle struct {
	config    *conf.Conf
	nodes     *node.Node
	core      *core.V2Core
	reloadCh  chan reloadEvent.Event
	signature string
}

type reloadFailure struct {
	action string
	fatal  bool
	err    error
}

type reloadOutcome struct {
	result string
	action string
}

func (e *reloadFailure) Error() string {
	if e.err == nil {
		return e.action
	}
	return e.err.Error()
}

func (e *reloadFailure) Unwrap() error {
	return e.err
}

func reload(config string, current **runtimeBundle, ev reloadEvent.Event) (*reloadOutcome, error) {
	if current == nil || *current == nil {
		return nil, &reloadFailure{action: "runtime_missing", fatal: true, err: fmt.Errorf("current runtime is nil")}
	}
	oldRuntime := *current
	newConf, err := loadConfig(config)
	if err != nil {
		return nil, &reloadFailure{action: "keep_old_runtime", err: fmt.Errorf("load new config: %w", err)}
	}
	localRuntimeChanged := !sameLocalRuntimeConfig(oldRuntime.config, newConf)
	prepared, err := newPreparedRuntime(newConf, oldRuntime.reloadCh)
	if err != nil {
		return nil, &reloadFailure{
			action: "prepare_failed",
			fatal:  isFatalReloadFailure(ev.Reason, localRuntimeChanged),
			err:    fmt.Errorf("prepare new runtime: %w", err),
		}
	}
	runtimeChanged := !sameRuntimeConfig(oldRuntime, prepared)
	if !runtimeChanged {
		_ = prepared.close("runtime_unchanged", log.Fields{"reason": ev.ReasonString()})
		applyLogConfig(prepared.config.LogConfig)
		*current = oldRuntime
		runtime.GC()
		return &reloadOutcome{result: "skipped", action: "runtime_unchanged"}, nil
	}

	if err := oldRuntime.closeNodes(); err != nil {
		_ = prepared.close("reload_prepare_discard", log.Fields{"reason": "old_runtime_close_failed"})
		_ = oldRuntime.closeCore("reload", log.Fields{"reason": "old_runtime_close_failed"})
		return nil, &reloadFailure{action: "old_runtime_close_failed", fatal: true, err: fmt.Errorf("close old nodes: %w", err)}
	}
	closeFields := reloadEventFields(ev)
	closeFields["reload_reason"] = ev.ReasonString()
	if err := oldRuntime.closeCore("reload", closeFields); err != nil {
		_ = prepared.close("reload_prepare_discard", log.Fields{"reason": "old_core_close_failed"})
		return nil, &reloadFailure{action: "old_core_close_failed", fatal: true, err: fmt.Errorf("close old core: %w", err)}
	}

	if err := prepared.start(); err != nil {
		_ = prepared.close("reload_failed", log.Fields{"reason": "new_runtime_start_failed"})
		return nil, &reloadFailure{action: "start_failed", fatal: true, err: fmt.Errorf("start new runtime: %w", err)}
	}
	applyLogConfig(prepared.config.LogConfig)
	*current = prepared
	runtime.GC()
	return &reloadOutcome{result: "success", action: "runtime_changed"}, nil
}

func newPreparedRuntime(c *conf.Conf, reloadCh chan reloadEvent.Event) (*runtimeBundle, error) {
	runtimeConfig := cloneConfig(c)
	nodes, err := node.New(runtimeConfig.NodeConfigs)
	if err != nil {
		return nil, fmt.Errorf("get node info: %w", err)
	}
	log.Info("Got nodes info from server")
	v2core := core.New(runtimeConfig)
	v2core.ReloadCh = reloadCh
	if err := v2core.Prepare(nodes.NodeInfos); err != nil {
		return nil, fmt.Errorf("prepare core: %w", err)
	}
	if err := nodes.Prepare(v2core); err != nil {
		_ = v2core.Close()
		return nil, fmt.Errorf("prepare nodes: %w", err)
	}
	return &runtimeBundle{
		config:    runtimeConfig,
		nodes:     nodes,
		core:      v2core,
		reloadCh:  reloadCh,
		signature: buildRuntimeSignature(runtimeConfig, nodes.NodeInfos),
	}, nil
}

func loadConfig(config string) (*conf.Conf, error) {
	newConf := conf.New()
	if err := newConf.LoadFromPath(config); err != nil {
		return nil, err
	}
	return newConf, nil
}

func (b *runtimeBundle) start() error {
	if b == nil || b.core == nil || b.nodes == nil {
		return fmt.Errorf("runtime is incomplete")
	}
	if err := b.core.StartPrepared(); err != nil {
		return fmt.Errorf("start core: %w", err)
	}
	if err := b.nodes.StartPrepared(b.core); err != nil {
		return fmt.Errorf("start nodes: %w", err)
	}
	return nil
}

func (b *runtimeBundle) close(reason string, fields log.Fields) error {
	if b == nil {
		return nil
	}
	var err error
	if closeErr := b.closeNodes(); closeErr != nil {
		err = closeErr
	}
	if closeErr := b.closeCore(reason, fields); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (b *runtimeBundle) closeNodes() error {
	if b == nil || b.nodes == nil {
		return nil
	}
	return b.nodes.Close()
}

func (b *runtimeBundle) closeCore(reason string, fields log.Fields) error {
	if b == nil || b.core == nil {
		return nil
	}
	return b.core.CloseWithReason(reason, fields)
}

func (b *runtimeBundle) ActiveLinkCount() int {
	if b == nil || b.core == nil {
		return 0
	}
	return b.core.ActiveLinkCount()
}

func setLogLevel(level string) {
	switch level {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn", "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	}
}

func applyLogConfig(c conf.LogConfig) {
	setLogLevel(c.Level)
	if c.Output == "" {
		setLogOutput(os.Stdout)
		return
	}
	f, err := os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.WithField("err", err).Error("Open log file failed, using stdout instead")
		setLogOutput(os.Stdout)
		return
	}
	setLogOutput(f)
}

func setLogOutput(writer *os.File) {
	oldWriter, ok := log.StandardLogger().Out.(*os.File)
	if ok && oldWriter != os.Stdout && oldWriter != os.Stderr && oldWriter != writer {
		_ = oldWriter.Close()
	}
	log.SetOutput(writer)
}

func cloneConfig(c *conf.Conf) *conf.Conf {
	if c == nil {
		return nil
	}
	cloned := *c
	cloned.NodeConfigs = make([]conf.NodeConfig, len(c.NodeConfigs))
	for i := range c.NodeConfigs {
		cloned.NodeConfigs[i] = c.NodeConfigs[i]
		if c.NodeConfigs[i].RetryCount != nil {
			retryCount := *c.NodeConfigs[i].RetryCount
			cloned.NodeConfigs[i].RetryCount = &retryCount
		}
	}
	return &cloned
}

type runtimeSignatureData struct {
	Nodes        []runtimeSignatureNode `json:"nodes"`
	Fingerprints []string               `json:"fingerprints,omitempty"`
}

type runtimeSignatureNode struct {
	APIHost    string `json:"api_host"`
	NodeID     int    `json:"node_id"`
	Key        string `json:"api_key"`
	Timeout    int    `json:"timeout"`
	RetryCount int    `json:"retry_count"`
}

func buildRuntimeSignature(c *conf.Conf, infos []*panel.NodeInfo) string {
	signature := runtimeSignatureData{
		Nodes:        buildRuntimeSignatureNodes(c),
		Fingerprints: make([]string, 0, len(infos)),
	}
	for _, info := range infos {
		if info == nil {
			signature.Fingerprints = append(signature.Fingerprints, "")
			continue
		}
		signature.Fingerprints = append(signature.Fingerprints, info.CoreFingerprint())
	}
	data, err := json.Marshal(signature)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func buildRuntimeSignatureNodes(c *conf.Conf) []runtimeSignatureNode {
	if c == nil {
		return nil
	}
	nodes := make([]runtimeSignatureNode, len(c.NodeConfigs))
	for i, nodeConfig := range c.NodeConfigs {
		retryCount := conf.DefaultNodeRetryCount
		if nodeConfig.RetryCount != nil {
			retryCount = *nodeConfig.RetryCount
		}
		timeout := nodeConfig.Timeout
		if timeout <= 0 {
			timeout = conf.DefaultNodeTimeout
		}
		nodes[i] = runtimeSignatureNode{
			APIHost:    nodeConfig.APIHost,
			NodeID:     nodeConfig.NodeID,
			Key:        nodeConfig.Key,
			Timeout:    timeout,
			RetryCount: retryCount,
		}
	}
	return nodes
}

func sameRuntimeConfig(oldRuntime, newRuntime *runtimeBundle) bool {
	if oldRuntime == nil || newRuntime == nil {
		return false
	}
	return oldRuntime.signature == newRuntime.signature
}

func sameLocalRuntimeConfig(oldConfig, newConfig *conf.Conf) bool {
	return buildRuntimeSignature(oldConfig, nil) == buildRuntimeSignature(newConfig, nil)
}

func isFatalReloadFailure(reason reloadEvent.Reason, changed bool) bool {
	return changed || reason == reloadEvent.ReasonNodeConfigChanged || reason == reloadEvent.ReasonCertMetadataChange
}

func reloadEventFields(ev reloadEvent.Event) log.Fields {
	fields := log.Fields{
		"reason": ev.ReasonString(),
	}
	if ev.NodeTag != "" {
		fields["tag"] = ev.NodeTag
	}
	if ev.TaskName != "" {
		fields["task"] = ev.TaskName
	}
	for key, value := range ev.Details {
		if key == "" {
			continue
		}
		fields["detail_"+key] = value
	}
	return fields
}
